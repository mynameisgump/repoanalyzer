// Package remote fetches pull request stats from GitHub via the gh CLI,
// which reuses the user's existing login instead of needing a token here.
package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/mynameisgump/repoanalyzer/internal/gitx"
)

type PRAuthor struct {
	Login  string `json:"login"`
	Merged int    `json:"merged"`
}

type PRStats struct {
	Repo   string `json:"repo"`
	Open   int    `json:"open"`
	Merged int    `json:"merged"`
	Closed int    `json:"closed"` // closed without merging
	// Computed over the most recent merged PRs (Sample of them).
	MedianHoursToMerge float64    `json:"median_hours_to_merge"`
	Sample             int        `json:"sample"`
	TopAuthors         []PRAuthor `json:"top_authors"`
}

const query = `query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    open: pullRequests(states: OPEN) { totalCount }
    merged: pullRequests(states: MERGED) { totalCount }
    closed: pullRequests(states: CLOSED) { totalCount }
    recent: pullRequests(states: MERGED, first: 100, orderBy: {field: CREATED_AT, direction: DESC}) {
      nodes { createdAt mergedAt author { login } }
    }
  }
}`

type response struct {
	Data struct {
		Repository struct {
			Open   struct{ TotalCount int } `json:"open"`
			Merged struct{ TotalCount int } `json:"merged"`
			Closed struct{ TotalCount int } `json:"closed"`
			Recent struct {
				Nodes []struct {
					CreatedAt time.Time `json:"createdAt"`
					MergedAt  time.Time `json:"mergedAt"`
					Author    *struct {
						Login string `json:"login"`
					} `json:"author"`
				} `json:"nodes"`
			} `json:"recent"`
		} `json:"repository"`
	} `json:"data"`
}

// FetchPRs resolves the GitHub repo for dir and queries its pull request stats.
func FetchPRs(ctx context.Context, dir string) (*PRStats, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, errors.New("gh CLI not found; install it (brew install gh) to see PR stats")
	}
	nwo, err := gitx.Run(ctx, dir, "gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
	if err != nil {
		return nil, fmt.Errorf("no GitHub repo found for this checkout: %w", err)
	}
	owner, name, ok := strings.Cut(nwo, "/")
	if !ok {
		return nil, fmt.Errorf("unexpected repo name %q", nwo)
	}
	out, err := gitx.Run(ctx, dir, "gh", "api", "graphql",
		"-f", "query="+query, "-f", "owner="+owner, "-f", "name="+name)
	if err != nil {
		return nil, err
	}
	var resp response
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return nil, fmt.Errorf("parsing GitHub response: %w", err)
	}

	r := resp.Data.Repository
	s := &PRStats{
		Repo:   nwo,
		Open:   r.Open.TotalCount,
		Merged: r.Merged.TotalCount,
		Closed: r.Closed.TotalCount,
	}
	var hours []float64
	byAuthor := map[string]int{}
	for _, n := range r.Recent.Nodes {
		hours = append(hours, n.MergedAt.Sub(n.CreatedAt).Hours())
		login := "ghost" // GitHub's name for deleted accounts
		if n.Author != nil {
			login = n.Author.Login
		}
		byAuthor[login]++
	}
	s.Sample = len(hours)
	s.MedianHoursToMerge = median(hours)
	for login, n := range byAuthor {
		s.TopAuthors = append(s.TopAuthors, PRAuthor{Login: login, Merged: n})
	}
	sort.Slice(s.TopAuthors, func(i, j int) bool {
		if s.TopAuthors[i].Merged != s.TopAuthors[j].Merged {
			return s.TopAuthors[i].Merged > s.TopAuthors[j].Merged
		}
		return s.TopAuthors[i].Login < s.TopAuthors[j].Login
	})
	return s, nil
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sort.Float64s(xs)
	m := len(xs) / 2
	if len(xs)%2 == 1 {
		return xs[m]
	}
	return (xs[m-1] + xs[m]) / 2
}
