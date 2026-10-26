package stats

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mynameisgump/repoanalyzer/internal/gitlog"
)

func TestCompute(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.Local)
	day := func(n int) time.Time { return now.AddDate(0, 0, -n) }
	commits := []gitlog.Commit{
		{AuthorName: "Ada", When: day(0), Subject: "Fix thing (#42)",
			Files: []gitlog.FileChange{{Path: "a.go", Added: 5, Deleted: 1}}},
		{AuthorName: "ada", When: day(10), Subject: "Merge pull request #41 from x/y"},
		{AuthorName: "Bob", When: day(200), Subject: "Initial",
			Files: []gitlog.FileChange{{Path: "a.go", Added: 10}, {Path: "gone.go", Added: 3}}},
	}
	s := Compute(commits, now, Paths{Exists: func(p string) bool { return p != "gone.go" }}, nil)

	if s.Commits != 3 || s.Added != 18 || s.Deleted != 1 {
		t.Errorf("totals: %+v", s)
	}
	if len(s.Authors) != 2 || s.Authors[0].Name != "Ada" || s.Authors[0].Commits != 2 {
		t.Errorf("authors should merge case-insensitively: %+v", s.Authors)
	}
	if s.ActiveAuthors90d != 1 {
		t.Errorf("active = %d, want 1", s.ActiveAuthors90d)
	}
	if s.PRsInHistory != 2 {
		t.Errorf("prs = %d, want 2", s.PRsInHistory)
	}
	if len(s.HotFiles) != 1 || s.HotFiles[0].Path != "a.go" || s.HotFiles[0].Commits != 2 {
		t.Errorf("hot files: %+v", s.HotFiles)
	}
	if !s.First.Equal(day(200)) || !s.Last.Equal(day(0)) {
		t.Errorf("range: %v .. %v", s.First, s.Last)
	}

	total := 0
	for i, b := range s.Weekly {
		total += b.Commits
		if b.Start.Weekday() != time.Monday {
			t.Errorf("bucket %d starts on %v", i, b.Start.Weekday())
		}
		if i > 0 && b.Start.Sub(s.Weekly[i-1].Start) < 6*24*time.Hour {
			t.Errorf("buckets not contiguous weeks at %d", i)
		}
	}
	if total != 3 || len(s.Weekly) < 28 {
		t.Errorf("weekly: %d buckets, %d commits", len(s.Weekly), total)
	}

	// Weeks run contiguously from Bob's commit to Ada's, zero-filled between,
	// and line up with Weekly.
	weeks := s.AuthorsWeekly
	if len(weeks) != len(s.Weekly) {
		t.Fatalf("weeks = %d, want %d like Weekly", len(weeks), len(s.Weekly))
	}
	for i, wk := range weeks {
		if !wk.Start.Equal(s.Weekly[i].Start) {
			t.Errorf("week %d starts %v, Weekly says %v", i, wk.Start, s.Weekly[i].Start)
		}
	}
	if first := weeks[0]; first.Active != 1 || first.Commits("Bob") != 1 {
		t.Errorf("first week = %+v", first)
	}
	if last := weeks[len(weeks)-1]; last.Active != 1 || last.Commits("Ada") != 1 || last.Commits("ada") != 0 {
		t.Errorf("last week should credit Ada by her display name: %+v", last)
	}
	if quiet := weeks[5]; quiet.Active != 0 || quiet.ByAuthor != nil {
		t.Errorf("quiet week = %+v", quiet)
	}
}

func TestComputeEmpty(t *testing.T) {
	s := Compute(nil, time.Now(), Paths{}, nil)
	if s.Commits != 0 || s.Weekly != nil || len(s.Authors) != 0 {
		t.Errorf("empty: %+v", s)
	}
}

func TestLinesOverTime(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.Local)
	commits := []gitlog.Commit{
		{When: now, Files: []gitlog.FileChange{{Path: "a.go", Added: 30, Deleted: 10}, {Path: "go.sum", Added: 500}}},
		{When: now.AddDate(0, 0, -14), Files: []gitlog.FileChange{{Path: "a.go", Added: 100}}},
	}
	skip := func(p string) bool { return p == "go.sum" }

	got := LinesOverTime(commits, 120, skip)
	want := []int{100, 100, 120} // week 1 adds 100, week 2 is quiet, week 3 nets +20
	if len(got) != len(want) {
		t.Fatalf("got %d points, want %d", len(got), len(want))
	}
	for i, p := range got {
		if p.Lines != want[i] || p.Start.Weekday() != time.Monday {
			t.Errorf("point %d = %+v, want %d lines", i, p, want[i])
		}
	}

	// History the diffs can't explain must not produce negative counts.
	if p := LinesOverTime(commits, 10, skip); p[0].Lines != 0 || p[1].Lines != 0 {
		t.Errorf("expected clamp to 0, got %+v", p)
	}
}

func TestPRSizes(t *testing.T) {
	jan := time.Date(2026, 1, 10, 12, 0, 0, 0, time.Local)
	feb := time.Date(2026, 2, 3, 12, 0, 0, 0, time.Local)
	mainline := []gitlog.Commit{
		{When: feb, AuthorName: "Merger", Parents: []string{"m1", "tip"}, Subject: "Merge pull request #3 from x/y",
			Files: []gitlog.FileChange{{Path: "a.go", Added: 40, Deleted: 20}, {Path: "go.sum", Added: 900}}},
		{When: jan.AddDate(0, 0, 5), AuthorName: "ada", Subject: "Fix bug (#2)", Files: []gitlog.FileChange{{Path: "a.go", Added: 5, Deleted: 5}}},
		{When: jan, AuthorName: "Bob", Subject: "Big feature (#1)", Files: []gitlog.FileChange{{Path: "b.go", Added: 30}}},
		{When: jan, AuthorName: "Bob", Subject: "Direct push, not a PR", Files: []gitlog.FileChange{{Path: "c.go", Added: 1000}}},
	}
	names := map[string]string{"tip": "Ada"}
	got := PRSizes(mainline, nil, names, func(p string) bool { return p == "go.sum" })

	if got.PRs != 3 || got.MedianLines != 30 || got.MeanLines != 100.0/3 {
		t.Errorf("summary = %+v", got)
	}
	if len(got.Monthly) != 2 {
		t.Fatalf("monthly = %+v", got.Monthly)
	}
	if m := got.Monthly[0]; m.Start.Month() != time.January || m.PRs != 2 || m.AvgLines != 20 {
		t.Errorf("january = %+v", m)
	}
	if m := got.Monthly[1]; m.Start.Month() != time.February || m.PRs != 1 || m.AvgLines != 60 {
		t.Errorf("february = %+v", m)
	}
	// The merge goes to the PR branch's author, not Merger, and names merge case-insensitively.
	if len(got.ByAuthor) != 2 {
		t.Fatalf("by author = %+v", got.ByAuthor)
	}
	if a := got.ByAuthor[0]; a.Name != "Ada" || a.PRs != 2 || a.MeanLines != 35 || a.MedianLines != 60 {
		t.Errorf("ada = %+v", a)
	}
	if b := got.ByAuthor[1]; b.Name != "Bob" || b.PRs != 1 || b.MeanLines != 30 {
		t.Errorf("bob = %+v", b)
	}
	if empty := PRSizes(nil, nil, nil, nil); empty.PRs != 0 || empty.Monthly != nil {
		t.Errorf("empty = %+v", empty)
	}
}

func TestTestShares(t *testing.T) {
	jan := time.Date(2026, 1, 10, 12, 0, 0, 0, time.Local)
	feb := time.Date(2026, 2, 3, 12, 0, 0, 0, time.Local)
	commits := []gitlog.Commit{
		{AuthorName: "Ada", When: feb, Files: []gitlog.FileChange{{Path: "a.go", Added: 30}, {Path: "a_test.go", Added: 10}}},
		{AuthorName: "Ada", When: jan, Files: []gitlog.FileChange{{Path: "b.go", Added: 60}}},
		{AuthorName: "Bob", When: jan, Files: []gitlog.FileChange{{Path: "b_test.go", Added: 20}, {Path: "go.sum", Added: 900}}},
		{AuthorName: "Bob", When: jan, Files: []gitlog.FileChange{{Path: "go.sum", Added: 5}}}, // lockfile only: not counted
		{AuthorName: "Bob", When: jan, Subject: "Merge branch 'x'"},                            // no diff: not counted
	}
	s := Compute(commits, jan, Paths{
		IsTest: func(p string) bool { return strings.HasSuffix(p, "_test.go") },
		Skip:   func(p string) bool { return p == "go.sum" },
	}, nil)

	if want := (TestShare{Commits: 3, TestCommits: 2, Added: 120, TestAdded: 30}); s.Tests != want {
		t.Errorf("overall = %+v, want %+v", s.Tests, want)
	}
	if len(s.TestsMonthly) != 2 || s.TestsMonthly[0].Start.Month() != time.January ||
		s.TestsMonthly[0].TestCommits != 1 || s.TestsMonthly[1].Commits != 1 {
		t.Errorf("monthly = %+v", s.TestsMonthly)
	}
	// Skipped files are left out of every file stat, but their commits still count.
	if bob := s.Authors[0]; bob.Name != "Bob" || bob.Commits != 3 || bob.Added != 20 {
		t.Errorf("bob = %+v, want 3 commits and 20 lines added (go.sum skipped)", bob)
	}
	for _, f := range s.HotFiles {
		if f.Path == "go.sum" {
			t.Errorf("skipped go.sum is a hot file")
		}
		if f.Test != strings.HasSuffix(f.Path, "_test.go") {
			t.Errorf("%s: test = %v", f.Path, f.Test)
		}
	}
}

// Hot files keep the top of tests and non-tests separately: sorted by path, the test
// file would fall outside an overall top 50 and leave a tests-only list empty.
func TestHotFilesKeepTests(t *testing.T) {
	now := time.Now()
	var commits []gitlog.Commit
	for i := range hotFileLimit + 10 {
		files := []gitlog.FileChange{{Path: fmt.Sprintf("f%02d.go", i)}}
		if i == 0 {
			files = append(files, gitlog.FileChange{Path: "x_test.go"})
		}
		for range 3 {
			commits = append(commits, gitlog.Commit{When: now, Files: files})
		}
	}
	s := Compute(commits, now, Paths{IsTest: func(p string) bool { return strings.HasSuffix(p, "_test.go") }}, nil)
	tests := 0
	for _, f := range s.HotFiles {
		if f.Test {
			tests++
		}
	}
	if len(s.HotFiles) != hotFileLimit+1 || tests != 1 {
		t.Errorf("kept %d files, %d tests", len(s.HotFiles), tests)
	}
}

// People are merged by name or email, transitively, and shown under the name
// they used most; windows reuse the whole history's identities.
func TestIdentify(t *testing.T) {
	now := time.Now()
	commit := func(name, email string, subject string) gitlog.Commit {
		return gitlog.Commit{AuthorName: name, AuthorEmail: email, When: now, Subject: subject}
	}
	commits := []gitlog.Commit{ // newest first, like git log
		commit("Dax", "m@y.io", ""),
		commit("Dax", "d@x.io", ""),
		commit("Dax Raad", "d@x.io", ""),
		commit("Dax Raad", "d@x.io", ""),
		commit("Dax Raad", "d@x.io", ""),
		commit("Ada", "ada@work.com", ""),
		commit("ada", "ada@home.net", ""),
		commit("Bob", "", ""),
		commit("bob", "", ""),
		commit("Eve", "eve@x.io", ""),
	}
	s := Compute(commits, now, Paths{}, nil)
	if len(s.Authors) != 4 {
		t.Fatalf("authors = %+v", s.Authors)
	}
	// Dax Raad: 3 commits under that name, 2 as Dax, joined by d@x.io; m@y.io follows.
	dax := s.Authors[0]
	if dax.Name != "Dax Raad" || dax.Commits != 5 || dax.Email != "d@x.io" || !slices.Equal(dax.Aliases, []string{"Dax"}) {
		t.Errorf("dax = %+v", dax)
	}
	// Ada and ada share no email; names still merge case-insensitively, the
	// most recent spelling winning the tie, with no alias for a case variant.
	ada := s.Authors[1]
	if ada.Name != "Ada" || ada.Commits != 2 || ada.Email != "ada@work.com" || ada.Aliases != nil {
		t.Errorf("ada = %+v", ada)
	}
	if bob := s.Authors[2]; bob.Name != "Bob" || bob.Commits != 2 || bob.Email != "" {
		t.Errorf("bob = %+v (empty emails must not link people)", bob)
	}
	if last := s.AuthorsWeekly[len(s.AuthorsWeekly)-1]; last.Active != 4 || last.Commits("Dax Raad") != 5 {
		t.Errorf("weekly = %+v", last)
	}

	// A window holding only "Dax" commits still names the person Dax Raad
	// when given the whole history's identities, and not otherwise.
	ids := Identify(commits)
	window := commits[:2]
	if w := Compute(window, now, Paths{}, ids); w.Authors[0].Name != "Dax Raad" || w.Authors[0].Commits != 2 {
		t.Errorf("windowed = %+v", w.Authors)
	}
	if w := Compute(window, now, Paths{}, nil); w.Authors[0].Name != "Dax" {
		t.Errorf("on its own = %+v", w.Authors)
	}

	// PR sizes credit by the same names, so the table can join on them.
	prs := []gitlog.Commit{
		commit("Dax", "m@y.io", "Fix (#2)"),
		commit("ada", "ada@home.net", "Feat (#1)"),
	}
	by := PRSizes(prs, ids, nil, nil).ByAuthor
	if len(by) != 2 || by[0].Name != "Ada" || by[1].Name != "Dax Raad" {
		t.Errorf("pr authors = %+v", by)
	}
	if p := ids.Person(commit("Nobody", "new@x.io", "")); p.Name != "Nobody" || p.Email != "new@x.io" {
		t.Errorf("unknown author = %+v", p)
	}
	var none *Identities
	if none.Name(commit("Solo", "", "")) != "Solo" {
		t.Error("nil identities should pass names through")
	}
}

// Commits landed via a PR when they are a squash commit, a PR merge, or were
// brought in by one from the PR branch; commits a plain merge brought in, and
// commits straight onto the branch, were pushed directly.
func TestLandings(t *testing.T) {
	jan := time.Date(2026, 1, 10, 12, 0, 0, 0, time.Local)
	feb := time.Date(2026, 2, 3, 12, 0, 0, 0, time.Local)
	c := func(hash, author, subject string, when time.Time, parents ...string) gitlog.Commit {
		return gitlog.Commit{Hash: hash, AuthorName: author, Subject: subject, When: when, Parents: parents}
	}
	// main: m1 <- m2 (squash #1) <- m3 (merge #2 of b1..b2) <- m4 (plain merge of h1)
	// b1 <- b2 branch off m1 and merge main back in at b2; h1 branches off m3.
	mainline := []gitlog.Commit{
		c("m4", "Ada", "Merge branch 'hotfix'", feb, "m3", "h1"),
		c("m3", "Merger", "Merge pull request #2 from x/y", feb, "m2", "b2"),
		c("m2", "Bob", "Squash it (#1)", jan, "m1"),
		c("m1", "Ada", "Initial", jan),
	}
	branches := []gitlog.Commit{
		c("h1", "Ada", "Hotfix", feb, "m3"),
		c("b2", "Bob", "Merge main into feature", feb, "b1", "m2"),
		c("b1", "Bob", "Feature", jan, "m1"),
	}
	commits := append(slices.Clone(mainline), branches...)
	via := ViaPR(commits, mainline)
	for hash, want := range map[string]bool{"m1": false, "m2": true, "m3": true, "m4": false, "h1": false, "b1": true, "b2": true} {
		if via[hash] != want {
			t.Errorf("%s via PR = %v, want %v", hash, via[hash], want)
		}
	}

	l := Landings(commits, via, nil)
	if l.Commits != 7 || l.ViaPR != 4 || l.Direct() != 3 {
		t.Errorf("landings = %+v", l)
	}
	if len(l.Monthly) != 2 || l.Monthly[0].Commits != 3 || l.Monthly[0].ViaPR != 2 || l.Monthly[1].Commits != 4 || l.Monthly[1].ViaPR != 2 {
		t.Errorf("monthly = %+v", l.Monthly)
	}
	if len(l.ByAuthor) != 3 || l.ByAuthor[0].Name != "Ada" || l.ByAuthor[0].Direct() != 3 || l.ByAuthor[0].ViaPR != 0 ||
		l.ByAuthor[1].Name != "Bob" || l.ByAuthor[1].ViaPR != 3 || l.ByAuthor[2].Name != "Merger" || l.ByAuthor[2].ViaPR != 1 {
		t.Errorf("by author = %+v", l.ByAuthor)
	}

	// A window with the branch commits but not their merge still knows how they landed.
	if w := Landings(branches, via, nil); w.Commits != 3 || w.ViaPR != 2 {
		t.Errorf("windowed = %+v", w)
	}
	// Without a classification everything is a direct push.
	if d := Landings(commits, nil, nil); d.ViaPR != 0 || d.Direct() != 7 {
		t.Errorf("unclassified = %+v", d)
	}
	// History cut by --since: the merge is there but the branch isn't.
	if cut := ViaPR(mainline, mainline); !cut["m3"] || cut["b2"] {
		t.Errorf("cut = %v", cut)
	}
}
