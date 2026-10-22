// Package report gathers everything the dashboard and --json output need.
package report

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/mynameisgump/repoanalyzer/internal/gitlog"
	"github.com/mynameisgump/repoanalyzer/internal/gitx"
	"github.com/mynameisgump/repoanalyzer/internal/loc"
	"github.com/mynameisgump/repoanalyzer/internal/stats"
)

// Local is everything computed from the repo on disk (no network).
type Local struct {
	Repo    string        `json:"repo"`
	Root    string        `json:"root"`
	Branch  string        `json:"branch"`
	Since   string        `json:"since,omitempty"`
	LOC     loc.Result    `json:"loc"`
	History stats.Summary `json:"history"`

	// The parsed history History was computed from, kept so it can be recomputed
	// for a time window (see InWindow). Mainline is the first-parent history.
	Commits  []gitlog.Commit `json:"-"`
	Mainline []gitlog.Commit `json:"-"`
	tracked  map[string]bool
	names    map[string]string // commit hash -> author name
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
	l := &Local{
		Repo:     filepath.Base(root),
		Root:     root,
		Branch:   branch,
		Since:    since,
		LOC:      counted,
		Commits:  commits,
		Mainline: mainline,
		tracked:  tracked,
		names:    make(map[string]string, len(commits)),
	}
	// Lets PR sizes credit a merge commit to whoever wrote the PR branch, even
	// when the branch started before a time window does.
	for _, c := range commits {
		l.names[c.Hash] = c.AuthorName
	}
	l.History = l.compute(commits, mainline, time.Now())
	// Diffs count blank lines too, so anchor on total lines rather than code lines.
	l.History.LinesOverTime = stats.LinesOverTime(mainline, counted.Lines, loc.IsIgnored)
	return l, nil
}

// compute derives every history stat except LinesOverTime from the given commits.
func (l *Local) compute(commits, mainline []gitlog.Commit, now time.Time) stats.Summary {
	paths := stats.Paths{IsTest: loc.IsTest, Skip: loc.IsIgnored}
	if l.tracked != nil {
		paths.Exists = func(p string) bool { return l.tracked[p] && !loc.IsIgnored(p) }
	}
	s := stats.Compute(commits, now, paths)
	s.PRSizes = stats.PRSizes(mainline, l.names, loc.IsIgnored)
	return s
}

// Span is when the first and last changes landed on the branch, or zero times without history.
func (l *Local) Span() (first, last time.Time) {
	commits := l.Mainline
	if len(commits) == 0 {
		commits = l.Commits
	}
	for i, c := range commits {
		if i == 0 || c.When.Before(first) {
			first = c.When
		}
		if i == 0 || c.When.After(last) {
			last = c.When
		}
	}
	return first, last
}

// InWindow recomputes history stats for changes from..to (inclusive). Weekly
// commits are zero-filled across the whole window so repos line up. Lines over
// time is sliced rather than recomputed: it's anchored at today's line count.
// ActiveAuthors90d counts the 90 days before to.
func (l *Local) InWindow(from, to time.Time) stats.Summary {
	in := func(t time.Time) bool { return !t.Before(from) && !t.After(to) }
	var commits, mainline []gitlog.Commit
	for _, c := range l.Commits {
		if in(c.When) {
			commits = append(commits, c)
		}
	}
	for _, c := range l.Mainline {
		if in(c.When) {
			mainline = append(mainline, c)
		}
	}
	s := l.compute(commits, mainline, to)
	s.Weekly = stats.Weekly(commits, from, to)
	for _, p := range l.History.LinesOverTime {
		// Points are week starts; keep any week that overlaps the window.
		if !p.Start.After(to) && !p.Start.AddDate(0, 0, 7).Before(from) {
			s.LinesOverTime = append(s.LinesOverTime, p)
		}
	}
	return s
}
