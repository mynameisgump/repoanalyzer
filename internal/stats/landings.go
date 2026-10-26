package stats

import (
	"sort"
	"strings"
	"time"

	"github.com/mynameisgump/repoanalyzer/internal/gitlog"
)

// LandingStats splits commits by how they reached the branch: through a pull
// request, or pushed without one. A commit landed via a PR when it is a squash
// commit, a PR merge commit, or a commit the merge brought in from the PR
// branch; everything else, including commits brought in by a merge that wasn't
// a PR, counts as direct. PRs merged by rebasing leave no marker and look direct.
type LandingStats struct {
	Commits  int               `json:"commits"`
	ViaPR    int               `json:"via_pr"`
	Monthly  []MonthlyLandings `json:"monthly"`   // only months with commits
	ByAuthor []AuthorLandings  `json:"by_author"` // most commits first
}

// Direct is how many commits didn't come through a PR.
func (l LandingStats) Direct() int { return l.Commits - l.ViaPR }

// MonthlyLandings is one month's commits and how many came through PRs.
type MonthlyLandings struct {
	Start   time.Time `json:"start"`
	Commits int       `json:"commits"`
	ViaPR   int       `json:"via_pr"`
}

// AuthorLandings is one person's commits and how many came through PRs.
// Name matches Author.Name.
type AuthorLandings struct {
	Name    string `json:"name"`
	Commits int    `json:"commits"`
	ViaPR   int    `json:"via_pr"`
}

// Direct is how many of the author's commits didn't come through a PR.
func (a AuthorLandings) Direct() int { return a.Commits - a.ViaPR }

// ViaPR works out which of commits reached the branch through a pull request,
// by hash; see LandingStats. mainline is the branch's first-parent history
// (gitlog.ReadMainline): each PR merge on it is followed back through the
// PR branch's commits until the branch meets the mainline again. Work it out
// once over the whole history, since a time window can hold a branch's commits
// without the merge that landed them.
func ViaPR(commits, mainline []gitlog.Commit) map[string]bool {
	byHash := make(map[string]*gitlog.Commit, len(commits))
	for i := range commits {
		byHash[commits[i].Hash] = &commits[i]
	}
	onMain := make(map[string]bool, len(mainline))
	for _, c := range mainline {
		onMain[c.Hash] = true
	}
	via := map[string]bool{}
	for _, c := range mainline {
		if prNumber(c.Subject) == "" {
			continue
		}
		via[c.Hash] = true
		stack := append([]string(nil), c.Parents[min(len(c.Parents), 1):]...)
		for len(stack) > 0 {
			h := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if via[h] || onMain[h] {
				continue
			}
			bc, ok := byHash[h]
			if !ok {
				continue // before --since, or history we didn't read
			}
			via[h] = true
			stack = append(stack, bc.Parents...)
		}
	}
	return via
}

// Landings counts how commits landed, using viaPR from ViaPR (nil makes every
// commit direct) and ids to name authors (nil works them out from commits).
func Landings(commits []gitlog.Commit, viaPR map[string]bool, ids *Identities) LandingStats {
	if ids == nil {
		ids = Identify(commits)
	}
	var out LandingStats
	months := map[time.Time]*MonthlyLandings{}
	people := map[string]*AuthorLandings{}
	for _, c := range commits {
		pr := viaPR[c.Hash]
		local := c.When.Local()
		month := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, time.Local)
		mo := months[month]
		if mo == nil {
			mo = &MonthlyLandings{Start: month}
			months[month] = mo
		}
		name := ids.Name(c)
		p := people[strings.ToLower(name)]
		if p == nil {
			p = &AuthorLandings{Name: name}
			people[strings.ToLower(name)] = p
		}
		out.Commits++
		mo.Commits++
		p.Commits++
		if pr {
			out.ViaPR++
			mo.ViaPR++
			p.ViaPR++
		}
	}
	for _, mo := range months {
		out.Monthly = append(out.Monthly, *mo)
	}
	sort.Slice(out.Monthly, func(i, j int) bool { return out.Monthly[i].Start.Before(out.Monthly[j].Start) })
	for _, p := range people {
		out.ByAuthor = append(out.ByAuthor, *p)
	}
	sort.Slice(out.ByAuthor, func(i, j int) bool {
		if out.ByAuthor[i].Commits != out.ByAuthor[j].Commits {
			return out.ByAuthor[i].Commits > out.ByAuthor[j].Commits
		}
		return out.ByAuthor[i].Name < out.ByAuthor[j].Name
	})
	return out
}
