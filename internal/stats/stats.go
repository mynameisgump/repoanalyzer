// Package stats turns parsed commits into the numbers the dashboard shows.
// Everything here is pure so it can be tested without a repo.
package stats

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mynameisgump/repoanalyzer/internal/gitlog"
)

// Author is one contributor; see Identities for how commits are merged into one.
type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	// Aliases are other names the author committed under, most used first.
	Aliases []string  `json:"aliases,omitempty"`
	Commits int       `json:"commits"`
	Added   int       `json:"added"`
	Deleted int       `json:"deleted"`
	First   time.Time `json:"first"`
	Last    time.Time `json:"last"`
}

type File struct {
	Path    string `json:"path"`
	Commits int    `json:"commits"`
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
	Test    bool   `json:"test,omitempty"`
}

// TestShare is how much of a set of commits went to test files. Merge commits
// (which have no diff in this history) and skipped files like lockfiles are left out.
type TestShare struct {
	Commits     int `json:"commits"`      // commits that changed files
	TestCommits int `json:"test_commits"` // of those, the ones that changed a test file
	Added       int `json:"added"`        // lines added
	TestAdded   int `json:"test_added"`   // of those, lines added to test files
}

func (t *TestShare) add(o TestShare) {
	t.Commits += o.Commits
	t.TestCommits += o.TestCommits
	t.Added += o.Added
	t.TestAdded += o.TestAdded
}

// MonthlyTests is one month's TestShare.
type MonthlyTests struct {
	Start time.Time `json:"start"`
	TestShare
}

type Bucket struct {
	Start   time.Time `json:"start"`
	Commits int       `json:"commits"`
}

// WeeklyAuthors is one Monday-start week's contributor activity (local time).
type WeeklyAuthors struct {
	Start  time.Time `json:"start"`
	Active int       `json:"active"` // authors with a commit this week
	// ByAuthor is each active author's commits, most first; name matches Author.Name.
	ByAuthor []AuthorCommits `json:"by_author,omitempty"`
}

// AuthorCommits is one author's commit count.
type AuthorCommits struct {
	Name    string `json:"name"`
	Commits int    `json:"commits"`
}

// Commits returns an author's commits this week, by Author.Name.
func (wk WeeklyAuthors) Commits(name string) int {
	for _, a := range wk.ByAuthor {
		if a.Name == name {
			return a.Commits
		}
	}
	return 0
}

// Point is a week's end-of-week total line count.
type Point struct {
	Start time.Time `json:"start"`
	Lines int       `json:"lines"`
}

type Summary struct {
	Commits          int       `json:"commits"`
	Added            int       `json:"added"`
	Deleted          int       `json:"deleted"`
	First            time.Time `json:"first"`
	Last             time.Time `json:"last"`
	Authors          []Author  `json:"authors"` // most commits first
	ActiveAuthors90d int       `json:"active_authors_90d"`
	Weekly           []Bucket  `json:"weekly"` // contiguous, zero-filled, Monday-start
	// AuthorsWeekly is contributor activity per week, contiguous and
	// zero-filled from the first commit's week through the last's.
	AuthorsWeekly []WeeklyAuthors `json:"authors_weekly"`
	// LinesOverTime is filled in separately from mainline history; see LinesOverTime.
	LinesOverTime []Point `json:"lines_over_time"`
	HotFiles      []File  `json:"hot_files"`
	// Punchcard[weekday][hour] in local time; weekday 0 is Monday.
	Punchcard [7][24]int `json:"punchcard"`
	// PRsInHistory counts distinct PR numbers seen in merge / squash-merge subjects.
	// It is an offline estimate for when the GitHub API isn't available.
	PRsInHistory int `json:"prs_in_history"`
	// PRSizes is filled in separately from mainline history; see PRSizes.
	PRSizes PRSizeStats `json:"pr_sizes"`
	Tests   TestShare   `json:"tests"`
	// TestsMonthly is Tests per month, only for months with commits that changed files.
	TestsMonthly []MonthlyTests `json:"tests_monthly"`
}

// Paths tells Compute what a path is. Any field may be nil.
type Paths struct {
	Exists func(string) bool // still in the tree; hot files keep only these
	IsTest func(string) bool
	Skip   func(string) bool // left out of every file stat (e.g. lockfiles); the commit still counts
}

// PRSizeStats describes how big PRs were when they landed on the branch.
type PRSizeStats struct {
	PRs         int          `json:"prs"`
	MeanLines   float64      `json:"mean_lines"`
	MedianLines int          `json:"median_lines"`
	Monthly     []MonthlyAvg `json:"monthly"`   // only months in which PRs landed
	ByAuthor    []AuthorPRs  `json:"by_author"` // most PRs first
}

// AuthorPRs is one person's PR sizes. Name matches Author.Name case-insensitively.
type AuthorPRs struct {
	Name        string  `json:"name"`
	PRs         int     `json:"prs"`
	MeanLines   float64 `json:"mean_lines"`
	MedianLines int     `json:"median_lines"`
}

type MonthlyAvg struct {
	Start    time.Time `json:"start"`
	PRs      int       `json:"prs"`
	AvgLines float64   `json:"avg_lines"`
}

const hotFileLimit = 50

var prPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^Merge pull request #(\d+)`),
	regexp.MustCompile(`\(#(\d+)\)\s*$`),
}

// prNumber returns the PR number from a merge or squash-merge subject, or "".
func prNumber(subject string) string {
	for _, re := range prPatterns {
		if m := re.FindStringSubmatch(subject); m != nil {
			return m[1]
		}
	}
	return ""
}

// Compute aggregates commits. ids says who authored them; nil works them out
// from commits alone, but callers recomputing a window should pass the
// identities of the whole history so a person keeps one name in every window.
func Compute(commits []gitlog.Commit, now time.Time, paths Paths, ids *Identities) Summary {
	var s Summary
	if ids == nil {
		ids = Identify(commits)
	}
	authors := map[string]*Author{}
	files := map[string]*File{}
	months := map[time.Time]*TestShare{}
	prs := map[string]bool{}
	active := map[string]bool{}
	cutoff := now.AddDate(0, 0, -90)

	for _, c := range commits {
		s.Commits++
		if s.First.IsZero() || c.When.Before(s.First) {
			s.First = c.When
		}
		if c.When.After(s.Last) {
			s.Last = c.When
		}

		p := ids.Person(c)
		key := strings.ToLower(p.Name)
		a := authors[key]
		if a == nil {
			a = &Author{Name: p.Name, Email: p.Email, Aliases: p.Aliases, First: c.When, Last: c.When}
			authors[key] = a
		}
		a.Commits++
		if c.When.Before(a.First) {
			a.First = c.When
		}
		if c.When.After(a.Last) {
			a.Last = c.When
		}
		if c.When.After(cutoff) {
			active[key] = true
		}

		var share TestShare
		for _, f := range c.Files {
			if paths.Skip != nil && paths.Skip(f.Path) {
				continue
			}
			a.Added += f.Added
			a.Deleted += f.Deleted
			s.Added += f.Added
			s.Deleted += f.Deleted
			hf := files[f.Path]
			if hf == nil {
				hf = &File{Path: f.Path, Test: paths.IsTest != nil && paths.IsTest(f.Path)}
				files[f.Path] = hf
			}
			hf.Commits++
			hf.Added += f.Added
			hf.Deleted += f.Deleted

			share.Commits = 1
			share.Added += f.Added
			if hf.Test {
				share.TestCommits = 1
				share.TestAdded += f.Added
			}
		}

		local := c.When.Local()
		if share.Commits > 0 {
			s.Tests.add(share)
			month := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, time.Local)
			if months[month] == nil {
				months[month] = &TestShare{}
			}
			months[month].add(share)
		}
		s.Punchcard[(int(local.Weekday())+6)%7][local.Hour()]++

		if n := prNumber(c.Subject); n != "" {
			prs[n] = true
		}
	}

	for _, a := range authors {
		s.Authors = append(s.Authors, *a)
	}
	sort.Slice(s.Authors, func(i, j int) bool {
		if s.Authors[i].Commits != s.Authors[j].Commits {
			return s.Authors[i].Commits > s.Authors[j].Commits
		}
		return s.Authors[i].Name < s.Authors[j].Name
	})
	s.ActiveAuthors90d = len(active)
	s.PRsInHistory = len(prs)

	for month, t := range months {
		s.TestsMonthly = append(s.TestsMonthly, MonthlyTests{Start: month, TestShare: *t})
	}
	sort.Slice(s.TestsMonthly, func(i, j int) bool { return s.TestsMonthly[i].Start.Before(s.TestsMonthly[j].Start) })

	for _, f := range files {
		if paths.Exists == nil || paths.Exists(f.Path) {
			s.HotFiles = append(s.HotFiles, *f)
		}
	}
	sort.Slice(s.HotFiles, func(i, j int) bool {
		if s.HotFiles[i].Commits != s.HotFiles[j].Commits {
			return s.HotFiles[i].Commits > s.HotFiles[j].Commits
		}
		return s.HotFiles[i].Path < s.HotFiles[j].Path
	})
	// Keep the top of test and non-test files separately so either can be listed on its own.
	kept, tests, others := s.HotFiles[:0], 0, 0
	for _, f := range s.HotFiles {
		n := &others
		if f.Test {
			n = &tests
		}
		if *n < hotFileLimit {
			kept = append(kept, f)
			*n++
		}
	}
	s.HotFiles = kept

	if len(commits) > 0 {
		s.Weekly = Weekly(commits, s.First, s.Last)
		s.AuthorsWeekly = authorsWeekly(commits, authors, ids, s.First, s.Last)
	}
	return s
}

// authorsWeekly counts each author's commits per Monday-start week, zero-filled
// from first's week through last's. authors maps lowercased display names (as
// ids gives them) to their Author, so every week uses the same name.
func authorsWeekly(commits []gitlog.Commit, authors map[string]*Author, ids *Identities, first, last time.Time) []WeeklyAuthors {
	counts := map[time.Time]map[string]int{}
	for _, c := range commits {
		wk := weekStart(c.When)
		if counts[wk] == nil {
			counts[wk] = map[string]int{}
		}
		counts[wk][strings.ToLower(ids.Name(c))]++
	}
	var out []WeeklyAuthors
	end := weekStart(last)
	for wk := weekStart(first); !wk.After(end); wk = wk.AddDate(0, 0, 7) {
		w := WeeklyAuthors{Start: wk, Active: len(counts[wk])}
		for key, n := range counts[wk] {
			w.ByAuthor = append(w.ByAuthor, AuthorCommits{Name: authors[key].Name, Commits: n})
		}
		sort.Slice(w.ByAuthor, func(i, j int) bool {
			if w.ByAuthor[i].Commits != w.ByAuthor[j].Commits {
				return w.ByAuthor[i].Commits > w.ByAuthor[j].Commits
			}
			return w.ByAuthor[i].Name < w.ByAuthor[j].Name
		})
		out = append(out, w)
	}
	return out
}

func weekStart(t time.Time) time.Time {
	t = t.Local()
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

// Weekly counts commits per Monday-start week, zero-filled from first's week through last's.
func Weekly(commits []gitlog.Commit, first, last time.Time) []Bucket {
	counts := map[time.Time]int{}
	for _, c := range commits {
		counts[weekStart(c.When)]++
	}
	var out []Bucket
	end := weekStart(last)
	for w := weekStart(first); !w.After(end); w = w.AddDate(0, 0, 7) {
		out = append(out, Bucket{Start: w, Commits: counts[w]})
	}
	return out
}

// LinesOverTime estimates total lines at the end of each week by walking net
// line changes backwards from current, today's count. Anchoring at the present
// keeps the latest value exact and works with --since. mainline should come
// from gitlog.ReadMainline so changes are dated by when they landed.
// Files for which skip returns true don't count; skip may be nil.
func LinesOverTime(mainline []gitlog.Commit, current int, skip func(string) bool) []Point {
	if len(mainline) == 0 {
		return nil
	}
	first, last := mainline[0].When, mainline[0].When
	net := map[time.Time]int{}
	for _, c := range mainline {
		if c.When.Before(first) {
			first = c.When
		}
		if c.When.After(last) {
			last = c.When
		}
		w := weekStart(c.When)
		for _, f := range c.Files {
			if skip == nil || !skip(f.Path) {
				net[w] += f.Added - f.Deleted
			}
		}
	}
	var weeks []time.Time
	for w, end := weekStart(first), weekStart(last); !w.After(end); w = w.AddDate(0, 0, 7) {
		weeks = append(weeks, w)
	}
	out := make([]Point, len(weeks))
	running := current
	for i := len(weeks) - 1; i >= 0; i-- {
		out[i] = Point{Start: weeks[i], Lines: max(running, 0)}
		running -= net[weeks[i]]
	}
	return out
}

// PRSizes measures each PR as the diff it landed with: on a first-parent history
// (gitlog.ReadMainline) a merge commit's diff, or a squash commit's own diff, is the
// whole PR. Size is lines added + deleted, ignoring files for which skip is true.
// PRs merged by rebasing leave no marker in history and aren't counted.
//
// A squash commit is authored by the PR's author, but a merge commit by whoever
// merged it, so merges are credited to the author of the PR branch's tip (the
// second parent), looked up in names (hash -> display name). Merges whose tip
// isn't in names fall back to the merge commit's author, as ids names them;
// names and ids may be nil (see Compute).
func PRSizes(mainline []gitlog.Commit, ids *Identities, names map[string]string, skip func(string) bool) PRSizeStats {
	if ids == nil {
		ids = Identify(mainline)
	}
	type acc struct{ prs, lines int }
	months := map[time.Time]*acc{}
	var sizes []int
	type person struct {
		name  string
		sizes []int
	}
	people := map[string]*person{}
	for _, c := range mainline {
		if prNumber(c.Subject) == "" {
			continue
		}
		lines := 0
		for _, f := range c.Files {
			if skip == nil || !skip(f.Path) {
				lines += f.Added + f.Deleted
			}
		}
		sizes = append(sizes, lines)
		local := c.When.Local()
		month := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, time.Local)
		a := months[month]
		if a == nil {
			a = &acc{}
			months[month] = a
		}
		a.prs++
		a.lines += lines

		name := ids.Name(c)
		if len(c.Parents) > 1 && names[c.Parents[1]] != "" {
			name = names[c.Parents[1]]
		}
		key := strings.ToLower(name)
		p := people[key]
		if p == nil {
			p = &person{name: name}
			people[key] = p
		}
		p.sizes = append(p.sizes, lines)
	}
	if len(sizes) == 0 {
		return PRSizeStats{}
	}

	out := PRSizeStats{PRs: len(sizes)}
	out.MeanLines, out.MedianLines = meanMedian(sizes)
	for month, a := range months {
		out.Monthly = append(out.Monthly, MonthlyAvg{Start: month, PRs: a.prs, AvgLines: float64(a.lines) / float64(a.prs)})
	}
	sort.Slice(out.Monthly, func(i, j int) bool { return out.Monthly[i].Start.Before(out.Monthly[j].Start) })
	for _, p := range people {
		ap := AuthorPRs{Name: p.name, PRs: len(p.sizes)}
		ap.MeanLines, ap.MedianLines = meanMedian(p.sizes)
		out.ByAuthor = append(out.ByAuthor, ap)
	}
	sort.Slice(out.ByAuthor, func(i, j int) bool {
		if out.ByAuthor[i].PRs != out.ByAuthor[j].PRs {
			return out.ByAuthor[i].PRs > out.ByAuthor[j].PRs
		}
		return out.ByAuthor[i].Name < out.ByAuthor[j].Name
	})
	return out
}

// meanMedian sorts sizes in place; it must not be empty.
func meanMedian(sizes []int) (float64, int) {
	total := 0
	for _, n := range sizes {
		total += n
	}
	sort.Ints(sizes)
	return float64(total) / float64(len(sizes)), sizes[len(sizes)/2]
}
