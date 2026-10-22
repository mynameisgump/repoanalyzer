// Command repoanalyzer shows stats and charts for the git repo it's run in,
// or compares several repos side by side.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mynameisgump/repoanalyzer/internal/gitx"
	"github.com/mynameisgump/repoanalyzer/internal/remote"
	"github.com/mynameisgump/repoanalyzer/internal/report"
	"github.com/mynameisgump/repoanalyzer/internal/ui"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: repoanalyzer [flags] [repo ...]\n\n"+
			"With no repos, analyzes the current directory. Pass up to %d repos to compare them.\n\n", ui.MaxRepos)
		flag.PrintDefaults()
	}
	dir := flag.String("C", ".", "run as if started in this directory (when no repos are given)")
	since := flag.String("since", "", `only include history after this date (git syntax, e.g. "6 months ago" or 2025-01-01)`)
	asJSON := flag.Bool("json", false, "print stats as JSON instead of opening the dashboard")
	noRemote := flag.Bool("no-remote", false, "skip GitHub lookups (PR stats)")
	flag.Parse()

	dirs := flag.Args()
	if len(dirs) == 0 {
		dirs = []string{*dir}
	}
	if len(dirs) > ui.MaxRepos {
		fail(fmt.Errorf("can compare at most %d repos, got %d", ui.MaxRepos, len(dirs)))
	}

	ctx := context.Background()
	var roots []string
	seen := map[string]bool{}
	for _, d := range dirs {
		root, err := gitx.RepoRoot(ctx, d)
		if err != nil {
			fail(fmt.Errorf("%s is not inside a git repository", d))
		}
		if seen[root] {
			fail(fmt.Errorf("%s was given more than once", root))
		}
		seen[root] = true
		roots = append(roots, root)
	}

	if *asJSON {
		if err := printJSON(ctx, roots, *since, *noRemote); err != nil {
			fail(err)
		}
		return
	}

	p := tea.NewProgram(ui.New(ui.Options{Roots: roots, Since: *since, NoRemote: *noRemote}), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "repoanalyzer:", err)
	os.Exit(1)
}

type repoJSON struct {
	*report.Local
	PullRequests      *remote.PRStats `json:"pull_requests,omitempty"`
	PullRequestsError string          `json:"pull_requests_error,omitempty"`
}

// printJSON prints one object for a single repo, or an array when comparing.
func printJSON(ctx context.Context, roots []string, since string, noRemote bool) error {
	results := make([]repoJSON, len(roots))
	errs := make(chan error, len(roots))
	for i, root := range roots {
		go func() {
			r, err := collect(ctx, root, since, noRemote)
			results[i] = r
			errs <- err
		}()
	}
	for range roots {
		if err := <-errs; err != nil {
			return err
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if len(results) == 1 {
		return enc.Encode(results[0])
	}
	return enc.Encode(results)
}

func collect(ctx context.Context, root, since string, noRemote bool) (repoJSON, error) {
	var out repoJSON
	prDone := make(chan struct{})
	if !noRemote {
		go func() {
			defer close(prDone)
			prs, err := remote.FetchPRs(ctx, root)
			out.PullRequests = prs
			if err != nil {
				out.PullRequestsError = err.Error()
			}
		}()
	} else {
		close(prDone)
	}

	local, err := report.CollectLocal(ctx, root, since)
	<-prDone
	out.Local = local
	return out, err
}
