// Package report gathers everything the dashboard and --json output need.
package report

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/ethancrann/repoanalyzer/internal/gitlog"
	"github.com/ethancrann/repoanalyzer/internal/gitx"
	"github.com/ethancrann/repoanalyzer/internal/loc"
	"github.com/ethancrann/repoanalyzer/internal/stats"
)

// Local is everything computed from the repo on disk (no network).
type Local struct {
	Repo    string        `json:"repo"`
	Root    string        `json:"root"`
	Branch  string        `json:"branch"`
	Since   string        `json:"since,omitempty"`
	LOC     loc.Result    `json:"loc"`
	History stats.Summary `json:"history"`
}

// CollectLocal counts lines and reads history concurrently. root must be a repo top level.
func CollectLocal(ctx context.Context, root, since string) (*Local, error) {
	paths, err := loc.TrackedFiles(ctx, root)
	if err != nil {
		return nil, err
	}

	var (
		wg                  sync.WaitGroup
		counted             loc.Result
		commits, mainline   []gitlog.Commit
		logErr, mainlineErr error
	)
	wg.Add(3)
	go func() {
		defer wg.Done()
		counted = loc.Count(root, paths)
	}()
	go func() {
		defer wg.Done()
		commits, logErr = gitlog.Read(ctx, root, since)
	}()
	go func() {
		defer wg.Done()
		mainline, mainlineErr = gitlog.ReadMainline(ctx, root, since)
	}()
	wg.Wait()
	if logErr != nil {
		return nil, logErr
	}
	if mainlineErr != nil {
		return nil, mainlineErr
	}

	tracked := make(map[string]bool, len(paths))
	for _, p := range paths {
		tracked[p] = true
	}
	branch, _ := gitx.Git(ctx, root, "branch", "--show-current")
	if branch == "" {
		branch = "detached"
	}
	history := stats.Compute(commits, time.Now(), func(p string) bool { return tracked[p] && !loc.IsIgnored(p) })
	// Diffs count blank lines too, so anchor on total lines rather than code lines.
	history.LinesOverTime = stats.LinesOverTime(mainline, counted.Lines, loc.IsIgnored)
	// Lets PR sizes credit a merge commit to whoever wrote the PR branch.
	names := make(map[string]string, len(commits))
	for _, c := range commits {
		names[c.Hash] = c.AuthorName
	}
	history.PRSizes = stats.PRSizes(mainline, names, loc.IsIgnored)
	return &Local{
		Repo:    filepath.Base(root),
		Root:    root,
		Branch:  branch,
		Since:   since,
		LOC:     counted,
		History: history,
	}, nil
}
