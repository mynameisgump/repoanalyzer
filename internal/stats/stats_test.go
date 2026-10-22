package stats

import (
	"fmt"
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
	s := Compute(commits, now, Paths{Exists: func(p string) bool { return p != "gone.go" }})

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
}

func TestComputeEmpty(t *testing.T) {
	s := Compute(nil, time.Now(), Paths{})
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
	got := PRSizes(mainline, names, func(p string) bool { return p == "go.sum" })

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
	if empty := PRSizes(nil, nil, nil); empty.PRs != 0 || empty.Monthly != nil {
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
	})

	if want := (TestShare{Commits: 3, TestCommits: 2, Added: 120, TestAdded: 30}); s.Tests != want {
		t.Errorf("overall = %+v, want %+v", s.Tests, want)
	}
	if len(s.TestsMonthly) != 2 || s.TestsMonthly[0].Start.Month() != time.January ||
		s.TestsMonthly[0].TestCommits != 1 || s.TestsMonthly[1].Commits != 1 {
		t.Errorf("monthly = %+v", s.TestsMonthly)
	}
	for _, f := range s.HotFiles {
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
	s := Compute(commits, now, Paths{IsTest: func(p string) bool { return strings.HasSuffix(p, "_test.go") }})
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
