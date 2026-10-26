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
	// AllFiles keeps files that aren't code in every stat; see Skip.
	AllFiles bool `json:"all_files"`
	// NonCode counts the tracked files Skip leaves out as data, config, docs or assets.
	NonCode int `json:"non_code_files"`

	// The parsed history History was computed from, kept so it can be recomputed
	// for a time window (see InWindow). Mainline is the first-parent history.
	Commits  []gitlog.Commit `json:"-"`
	Mainline []gitlog.Commit `json:"-"`
	tracked  map[string]bool
	ids      *stats.Identities // who authored Commits, merged by name or email
	names    map[string]string // commit hash -> display name
}

// CollectLocal counts lines and reads history concurrently. root must be a repo top level.
// Unless allFiles is set, files that aren't code are left out; see Skip.
func CollectLocal(ctx context.Context, root, since string, allFiles bool) (*Local, error) {
	paths, err := loc.TrackedFiles(ctx, root)
	if err != nil {
		return nil, err
	}
	l := &Local{AllFiles: allFiles}
	var counted []string // what loc.Count reads; it reports lockfiles as skipped itself
	for _, p := range paths {
		if l.Skip(p) && !loc.IsIgnored(p) {
			l.NonCode++
			continue
		}
		counted = append(counted, p)
	}

	var (
		wg                  sync.WaitGroup
		lines               loc.Result
		commits, mainline   []gitlog.Commit
		logErr, mainlineErr error
	)
	wg.Add(3)
	go func() {
		defer wg.Done()
		lines = loc.Count(root, counted)
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
	l.Repo, l.Root, l.Branch, l.Since, l.LOC = filepath.Base(root), root, branch, since, lines
	l.Commits, l.Mainline, l.tracked = commits, mainline, tracked
	// Identities come from the whole history so a person keeps one name in
	// every time window. names lets PR sizes credit a merge commit to whoever
	// wrote the PR branch, even when the branch started before a window does.
	l.ids = stats.Identify(commits)
	l.names = make(map[string]string, len(commits))
	for _, c := range commits {
		l.names[c.Hash] = l.ids.Name(c)
	}
	l.History = l.compute(commits, mainline, time.Now())
	// Diffs count blank lines too, so anchor on total lines rather than code lines.
	l.History.LinesOverTime = stats.LinesOverTime(mainline, lines.Lines, l.Skip)
	return l, nil
}

// Skip reports whether a path is left out of every stat: lockfiles always, and
// unless AllFiles is set, files that aren't in a programming language (data,
// config, docs and assets like JSON, CSV, Markdown or Godot scenes; see loc.IsCode).
func (l *Local) Skip(p string) bool {
	return loc.IsIgnored(p) || !l.AllFiles && !loc.IsCode(loc.Detect(p))
}

// compute derives every history stat except LinesOverTime from the given commits.
func (l *Local) compute(commits, mainline []gitlog.Commit, now time.Time) stats.Summary {
	paths := stats.Paths{IsTest: loc.IsTest, Skip: l.Skip}
	if l.tracked != nil {
		paths.Exists = func(p string) bool { return l.tracked[p] }
	}
	s := stats.Compute(commits, now, paths, l.ids)
	s.PRSizes = stats.PRSizes(mainline, l.ids, l.names, l.Skip)
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
