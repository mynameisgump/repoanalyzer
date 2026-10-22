// Package stats turns parsed commits into the numbers the dashboard shows.
// Everything here is pure so it can be tested without a repo.
package stats

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ethancrann/repoanalyzer/internal/gitlog"
)

type Author struct {
	Name    string    `json:"name"`
	Email   string    `json:"email"`
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
}

type Bucket struct {
	Start   time.Time `json:"start"`
	Commits int       `json:"commits"`
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

const hotFileLimit = 25

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

// Compute aggregates commits. exists filters hot files down to paths still in the tree; nil keeps all.
func Compute(commits []gitlog.Commit, now time.Time, exists func(string) bool) Summary {
	var s Summary
	authors := map[string]*Author{}
	files := map[string]*File{}
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

		// Key on the (mailmapped) name: people often commit from several emails.
		key := strings.ToLower(c.AuthorName)
		a := authors[key]
		if a == nil {
			a = &Author{Name: c.AuthorName, Email: c.AuthorEmail, First: c.When, Last: c.When}
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

		for _, f := range c.Files {
			a.Added += f.Added
			a.Deleted += f.Deleted
			s.Added += f.Added
			s.Deleted += f.Deleted
			hf := files[f.Path]
			if hf == nil {
				hf = &File{Path: f.Path}
				files[f.Path] = hf
			}
			hf.Commits++
			hf.Added += f.Added
			hf.Deleted += f.Deleted
		}

		local := c.When.Local()
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

	for _, f := range files {
		if exists == nil || exists(f.Path) {
			s.HotFiles = append(s.HotFiles, *f)
		}
	}
	sort.Slice(s.HotFiles, func(i, j int) bool {
		if s.HotFiles[i].Commits != s.HotFiles[j].Commits {
			return s.HotFiles[i].Commits > s.HotFiles[j].Commits
		}
		return s.HotFiles[i].Path < s.HotFiles[j].Path
	})
	if len(s.HotFiles) > hotFileLimit {
		s.HotFiles = s.HotFiles[:hotFileLimit]
	}

	s.Weekly = weekly(commits, s.First, s.Last)
	return s
}

func weekStart(t time.Time) time.Time {
	t = t.Local()
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

func weekly(commits []gitlog.Commit, first, last time.Time) []Bucket {
	if len(commits) == 0 {
		return nil
	}
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
// second parent), looked up in names (hash -> author name). Merges whose tip
// isn't in names fall back to the merge commit's author; names may be nil.
func PRSizes(mainline []gitlog.Commit, names map[string]string, skip func(string) bool) PRSizeStats {
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

		name := c.AuthorName
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
