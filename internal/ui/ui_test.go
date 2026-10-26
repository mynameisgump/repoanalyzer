package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/mynameisgump/repoanalyzer/internal/gitlog"
	"github.com/mynameisgump/repoanalyzer/internal/loc"
	"github.com/mynameisgump/repoanalyzer/internal/remote"
	"github.com/mynameisgump/repoanalyzer/internal/report"
	"github.com/mynameisgump/repoanalyzer/internal/stats"
)

func sampleLocal() *report.Local {
	now := time.Now()
	var commits []gitlog.Commit
	for i := range 60 {
		files := []gitlog.FileChange{{Path: "some/really/long/path/to/a/file.go", Added: i, Deleted: 1}}
		if i%2 == 0 {
			files = append(files, gitlog.FileChange{Path: "some/really/long/path/to/a/file_test.go", Added: 3})
		}
		commits = append(commits, gitlog.Commit{
			AuthorName: []string{"Ada", "Bob", "A very long author name indeed"}[i%3],
			When:       now.AddDate(0, 0, -i*5),
			Subject:    "change (#1)",
			Files:      files,
		})
	}
	h := stats.Compute(commits, now, stats.Paths{IsTest: loc.IsTest}, nil)
	h.LinesOverTime = stats.LinesOverTime(commits, 25000, nil)
	h.PRSizes = stats.PRSizes(commits, nil, nil, nil)
	return &report.Local{
		Repo: "demo", Branch: "main",
		LOC: loc.Result{Files: 3, Code: 100,
			Languages: []loc.Language{{Name: "Go", Files: 2, Code: 90}, {Name: "YAML", Files: 1, Code: 10}},
			Largest:   []loc.File{{Path: "main.go", Lines: 60}, {Path: "main_test.go", Lines: 30, Test: true}}},
		History: h, Commits: commits, Mainline: commits,
	}
}

// Every tab should render at any terminal size without panicking or overflowing.
func TestRenderAllTabs(t *testing.T) {
	type repo struct {
		local  *report.Local
		err    error
		loaded bool
	}
	cases := []struct {
		name  string
		repos []repo
	}{
		{"sample", []repo{{local: sampleLocal(), loaded: true}}},
		{"empty repo", []repo{{local: &report.Local{Repo: "empty", Branch: "main"}, loaded: true}}},
		{"compare", []repo{{local: sampleLocal(), loaded: true}, {local: sampleLocal(), loaded: true}}},
		{"compare, one loading, one failed", []repo{
			{local: sampleLocal(), loaded: true}, {}, {err: errors.New("boom"), loaded: true},
		}},
	}
	for _, c := range cases {
		var roots []string
		for i := range c.repos {
			roots = append(roots, fmt.Sprintf("/src/repo%d", i))
		}
		for _, size := range [][2]int{{120, 40}, {80, 24}, {64, 12}, {63, 30}, {10, 3}} {
			var m tea.Model = New(Options{Roots: roots})
			m, _ = m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			if size[0] == 120 { // side by side: fits for two repos, falls back for three
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
			}
			for i, r := range c.repos {
				if r.loaded {
					m, _ = m.Update(localMsg{idx: i, data: r.local, err: r.err})
					m, _ = m.Update(prMsg{idx: i, err: errors.New("no remote")})
				}
			}
			for i := range tabNames {
				if i == 2 { // switch to a time window partway through so both paths render
					m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
				}
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{rune('1' + i)}})
				if i == activityTab {
					m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}})
				}
				if i == filesTab && size[0] == 80 { // also cover the filtered lists
					m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
				}
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
				out := m.View()
				if got := strings.Count(out, "\n") + 1; got != size[1] {
					t.Errorf("%s %v tab %d: view is %d lines, want %d", c.name, size, i+1, got, size[1])
				}
				for _, line := range strings.Split(out, "\n") {
					if w := lipgloss.Width(line); w > size[0] && size[0] >= 64 {
						t.Errorf("%s %v tab %d: line is %d wide: %q", c.name, size, i+1, w, line)
						break
					}
				}
			}
		}
	}
}

func TestDisplayNames(t *testing.T) {
	got := displayNames([]string{"/a/app", "/b/app", "/c/tool"})
	want := []string{"a/app", "b/app", "tool"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("displayNames = %v, want %v", got, want)
			break
		}
	}
}

func TestTimeWindow(t *testing.T) {
	key := func(m tea.Model, r rune) tea.Model {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		return m
	}
	load := func(roots int) tea.Model {
		var names []string
		for i := range roots {
			names = append(names, fmt.Sprintf("/src/repo%d", i))
		}
		var m tea.Model = New(Options{Roots: names, NoRemote: true})
		m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		for i := range roots {
			m, _ = m.Update(localMsg{idx: i, data: sampleLocal()})
		}
		return m
	}

	// Single repo: t cycles the presets and skips overlap; o does nothing.
	m := load(1)
	var seen []windowMode
	for range 5 {
		m = key(m, 't')
		seen = append(seen, m.(Model).window)
	}
	want := []windowMode{windowYear, windowHalf, windowQuarter, windowAll, windowYear}
	if fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Errorf("t cycle = %v, want %v", seen, want)
	}
	if m = key(m, 'o'); m.(Model).window != windowYear {
		t.Errorf("o should do nothing for one repo")
	}

	// Sample history spans ~300 days, so the last 3 months hold fewer commits than all time.
	m = key(key(m, 't'), 't')
	mm := m.(Model)
	if mm.window != windowQuarter || !mm.windowed() {
		t.Fatalf("window = %v", mm.window)
	}
	all, quarter := mm.repos[0].local.History.Commits, mm.repos[0].hist.Commits
	if quarter == 0 || quarter >= all {
		t.Errorf("3-month window has %d of %d commits", quarter, all)
	}
	if !strings.Contains(mm.header(), "last 3 months") {
		t.Errorf("header doesn't show the window: %q", mm.header())
	}

	// Comparing: o jumps to overlap and back to where it was.
	m = key(load(2), 't')
	if m = key(m, 'o'); m.(Model).window != windowOverlap || !m.(Model).windowed() {
		t.Errorf("o should switch to overlap")
	}
	if m = key(m, 'o'); m.(Model).window != windowYear {
		t.Errorf("o should return to the previous window, got %v", m.(Model).window)
	}

	// Overlap waits for every repo, and says so.
	var w tea.Model = New(Options{Roots: []string{"/a", "/b"}, NoRemote: true})
	w, _ = w.Update(localMsg{idx: 0, data: sampleLocal()})
	w = key(w, 'o')
	if wm := w.(Model); wm.windowed() || !strings.Contains(wm.windowLabel(), "waiting") {
		t.Errorf("overlap before load: windowed=%v label=%q", wm.windowed(), wm.windowLabel())
	}
}

func TestTruncatePath(t *testing.T) {
	p := "packages/api/controllers/users/userController.ts"
	for w, want := range map[int]string{
		60: p,
		44: "packages/api/…/users/userController.ts",
		38: "packages/api/…/users/userController.ts",
		32: "packages/api/…/userController.ts",
		28: "packages/…/userController.ts",
		22: "…/userController.ts",
		12: "…ntroller.ts",
	} {
		if got := truncatePath(p, w); got != want {
			t.Errorf("truncatePath(%d) = %q, want %q", w, got, want)
		}
		if got := truncatePath(p, w); lipgloss.Width(got) > w {
			t.Errorf("truncatePath(%d) = %q is too wide", w, got)
		}
	}
	if got := truncatePath("a-very-long-file-name.go", 10); got != "…e-name.go" {
		t.Errorf("no folders: %q", got)
	}
}

func TestChartHeight(t *testing.T) {
	key := func(m tea.Model, r rune) tea.Model {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		return m
	}
	var m tea.Model = New(Options{Roots: []string{"/src/repo"}, NoRemote: true})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	m, _ = m.Update(localMsg{idx: 0, data: sampleLocal()})
	lines := func(m tea.Model) int { return strings.Count(m.(Model).body(), "\n") + 1 }

	// Outside Activity the keys do nothing.
	if m = key(m, '+'); m.(Model).chartH() != defaultChartH {
		t.Errorf("+ on Overview changed the height to %d", m.(Model).chartH())
	}

	m = key(m, '3')
	before := lines(m)
	m = key(m, '+')
	if got := m.(Model).chartH(); got != defaultChartH+chartStep {
		t.Fatalf("height after + = %d", got)
	}
	// Every chart grows by one step.
	if got, want := lines(m)-before, len(activityCharts)*chartStep; got != want {
		t.Errorf("body grew by %d lines, want %d", got, want)
	}
	if !strings.Contains(m.(Model).header(), "↕ 16 rows") {
		t.Errorf("header doesn't show the height")
	}

	for range 20 {
		m = key(m, '+')
	}
	if got, want := m.(Model).chartH(), m.(Model).bodyHeight()-3; got != want {
		t.Errorf("max height = %d, want %d (one chart and its title fill the screen)", got, want)
	}
	for range 20 {
		m = key(m, '-')
	}
	if got := m.(Model).chartH(); got != minChartH {
		t.Errorf("min height = %d, want %d", got, minChartH)
	}

	// Growing the charts keeps the chart at the top of the screen in place.
	mm := m.(Model)
	mm.chartHeight = 0
	mm.render()
	mm.scroll = chartStarts(mm.content)[1] + 2 // a little way into the commits chart
	m = key(mm, '=')
	mm = m.(Model)
	if got, want := mm.scroll, chartStarts(mm.content)[1]+2; got != want {
		t.Errorf("scroll = %d, want %d", got, want)
	}
}

// Scrolling must reuse the rendered tab: re-rendering Activity takes milliseconds per keypress.
func TestScrollDoesNotRerender(t *testing.T) {
	var m tea.Model = New(Options{Roots: []string{"/src/repo"}, NoRemote: true})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	m, _ = m.Update(localMsg{idx: 0, data: sampleLocal()})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	before := m.(Model).content
	for _, k := range []tea.KeyType{tea.KeyDown, tea.KeyDown, tea.KeyPgDown, tea.KeyPgUp, tea.KeyUp, tea.KeyUp} {
		m, _ = m.Update(tea.KeyMsg{Type: k})
	}
	if after := m.(Model).content; &after[0] != &before[0] {
		t.Errorf("scrolling re-rendered the tab")
	}
	if m.(Model).scroll != 0 {
		t.Errorf("scroll = %d after returning to the top", m.(Model).scroll)
	}
	// Changing what's shown does re-render.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if after := m.(Model).content; &after[0] == &before[0] {
		t.Errorf("changing the time window didn't re-render")
	}
}

func TestExpandLists(t *testing.T) {
	now := time.Now()
	var commits []gitlog.Commit
	for i := range 60 {
		commits = append(commits, gitlog.Commit{AuthorName: fmt.Sprintf("Author %02d", i), When: now.AddDate(0, 0, -i)})
	}
	var langs []loc.Language
	for i := range 12 {
		langs = append(langs, loc.Language{Name: fmt.Sprintf("Lang%d", i), Code: 100 - i})
	}
	local := &report.Local{Repo: "big", Branch: "main", History: stats.Compute(commits, now, stats.Paths{}, nil), Commits: commits,
		LOC: loc.Result{Code: 1000, Languages: langs}}

	var m tea.Model = New(Options{Roots: []string{"/src/big"}, NoRemote: true})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = m.Update(localMsg{idx: 0, data: local})
	key := func(r rune) { m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}) }
	body := func() string { return strings.Join(m.(Model).content, "\n") }

	key('2')
	if !strings.Contains(body(), "+ 10 more · e to show all") || strings.Contains(body(), "Author 59") {
		t.Fatalf("authors should stop at 50 with a hint")
	}
	key('e')
	if !strings.Contains(body(), "Author 59") || !strings.Contains(body(), "e to show fewer") {
		t.Errorf("e should show every author")
	}

	// Expanding is per tab: Overview's languages are still cut.
	key('1')
	if !strings.Contains(body(), "+ 2 more · e to show all") {
		t.Errorf("overview should still be collapsed")
	}

	key('2')
	key('e')
	if strings.Contains(body(), "Author 59") || !strings.Contains(body(), "+ 10 more") {
		t.Errorf("a second e should collapse the authors again")
	}
}

func TestPRSort(t *testing.T) {
	authors := []stats.AuthorPRs{
		{Name: "Avg", PRs: 2, MeanLines: 900, MedianLines: 10},
		{Name: "Median", PRs: 5, MeanLines: 300, MedianLines: 280},
		{Name: "Busy", PRs: 40, MeanLines: 100, MedianLines: 60},
	}
	for by, want := range map[prSortKey]string{
		sortByMean:   "Avg Median Busy",
		sortByMedian: "Median Busy Avg",
		sortByPRs:    "Busy Median Avg",
	} {
		got := slices.Clone(authors)
		sortAuthorPRs(got, by)
		var names []string
		for _, a := range got {
			names = append(names, a.Name)
		}
		if strings.Join(names, " ") != want {
			t.Errorf("sort %d = %v, want %s", by, names, want)
		}
	}

	// s cycles the sort on Pull Requests and marks the sorted column; elsewhere it does nothing.
	local := sampleLocal()
	local.History.PRSizes.ByAuthor = authors
	var m tea.Model = New(Options{Roots: []string{"/src/repo"}, NoRemote: true})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = m.Update(localMsg{idx: 0, data: local})
	key := func(r rune) { m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}) }
	key('s')
	if m.(Model).prSort != sortByMean {
		t.Errorf("s on Overview changed the sort")
	}
	key('5')
	for _, want := range []string{"median lines ▼", "PRs ▼", "avg lines ▼"} {
		key('s')
		if body := strings.Join(m.(Model).content, "\n"); !strings.Contains(body, want) {
			t.Errorf("after s, header doesn't show %q", want)
		}
	}
}

func TestFileFilter(t *testing.T) {
	var m tea.Model = New(Options{Roots: []string{"/src/repo"}, NoRemote: true})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = m.Update(localMsg{idx: 0, data: sampleLocal()})
	key := func(r rune) { m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}) }
	// rows lists the Files tab's list rows (the ones with a bar), trailing padding removed.
	rows := func() []string {
		var out []string
		for _, line := range m.(Model).content {
			if strings.Contains(line, "█") {
				out = append(out, strings.TrimRight(line, " "))
			}
		}
		return out
	}
	// describe summarizes rows as "path[+test]" for comparison.
	describe := func() string {
		var out []string
		for _, r := range rows() {
			name := strings.Fields(r)[0]
			if strings.HasSuffix(r, " test") {
				name += "+test"
			}
			out = append(out, name)
		}
		return strings.Join(out, " ")
	}
	const (
		code = "some/really/long/path/to/a/file.go"
		test = "some/really/long/path/to/a/file_test.go"
	)

	key('f')
	if m.(Model).fileFilter != allFiles {
		t.Errorf("f on Overview changed the filter")
	}
	key('4')
	if got, want := describe(), code+" "+test+"+test main.go main_test.go+test"; got != want {
		t.Errorf("all files = %q, want %q", got, want)
	}

	key('f')
	if got, want := describe(), code+" main.go"; got != want {
		t.Errorf("non-test files = %q, want %q", got, want)
	}
	if !strings.Contains(m.(Model).header(), "⚑ non-test files") {
		t.Errorf("header doesn't show the filter")
	}

	key('f') // every row is a test, so none is tagged
	if got, want := describe(), test+" main_test.go"; got != want {
		t.Errorf("test files only = %q, want %q", got, want)
	}

	key('f')
	if m.(Model).fileFilter != allFiles || strings.Contains(m.(Model).header(), "⚑") {
		t.Errorf("f should cycle back to all files")
	}
}

func TestSideBySide(t *testing.T) {
	load := func(width, repos int) tea.Model {
		var roots []string
		for i := range repos {
			roots = append(roots, fmt.Sprintf("/src/repo%d", i))
		}
		var m tea.Model = New(Options{Roots: roots, NoRemote: true})
		m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
		for i := range repos {
			m, _ = m.Update(localMsg{idx: i, data: sampleLocal()})
		}
		return m
	}
	key := func(m tea.Model, r rune) tea.Model {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		return m
	}
	// headingRow is the first body line with a repo heading, and how many headings it holds.
	headingRow := func(m tea.Model) int {
		for _, line := range m.(Model).content {
			if n := strings.Count(line, "● repo"); n > 0 {
				return n
			}
		}
		return 0
	}

	m := key(load(160, 2), '4')
	if got := headingRow(m); got != 1 {
		t.Errorf("stacked: first heading row has %d repos, want 1", got)
	}
	m = key(m, 'c')
	if got := headingRow(m); got != 2 {
		t.Errorf("side by side: first heading row has %d repos, want 2", got)
	}
	if !strings.Contains(m.(Model).header(), "▥ side by side") {
		t.Errorf("header doesn't show side by side")
	}
	for _, line := range m.(Model).content {
		if w := lipgloss.Width(line); w > 160 {
			t.Errorf("line is %d wide: %q", w, line)
			break
		}
	}

	// Too narrow for columns: stays stacked and says so.
	m = key(key(load(90, 2), '4'), 'c')
	if got := headingRow(m); got != 1 || !strings.Contains(m.(Model).header(), "too narrow") {
		t.Errorf("narrow: %d repos in a row, header %q", got, m.(Model).header())
	}

	// A single repo ignores c.
	if m = key(load(160, 1), 'c'); m.(Model).sideBySide {
		t.Errorf("c with one repo turned on side by side")
	}
}

// Side by side, a section that's longer in one repo pushes the next section down in every column.
func TestSideBySideAlignsSections(t *testing.T) {
	big := sampleLocal()
	h := &big.History
	for i := range 30 {
		h.Authors = append(h.Authors, stats.Author{Name: fmt.Sprintf("Extra %02d", i), Commits: 1})
	}
	for i := range 30 {
		h.HotFiles = append(h.HotFiles, stats.File{Path: fmt.Sprintf("extra/file%02d.go", i), Commits: 1})
	}

	var m tea.Model = New(Options{Roots: []string{"/src/small", "/src/big"}, NoRemote: true})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	small := sampleLocal()
	small.History.PRSizes.Monthly = nil
	m, _ = m.Update(localMsg{idx: 0, data: small})
	m, _ = m.Update(localMsg{idx: 1, data: big})
	key := func(r rune) { m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}) }
	// aligned checks that title starts a line in both columns.
	aligned := func(title string) {
		t.Helper()
		for _, line := range m.(Model).content {
			if n := strings.Count(line, title); n > 0 {
				if n != 2 {
					t.Errorf("%q is on different lines in the two columns", title)
				}
				return
			}
		}
		t.Errorf("%q not found", title)
	}

	key('c')
	key('5') // one repo's PR size chart is a one-line placeholder, the other's a full chart
	aligned("PR size by author")
	key('4')
	aligned("Largest files")
}

// A repo's heading stays at the top of the body while its sections scroll past.
func TestStickyHeadings(t *testing.T) {
	var m tea.Model = New(Options{Roots: []string{"/src/one", "/src/two"}, NoRemote: true})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 12})
	m, _ = m.Update(localMsg{idx: 0, data: sampleLocal()})
	m, _ = m.Update(localMsg{idx: 1, data: sampleLocal()})
	key := func(r rune) { m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}) }
	// top is the first body line on screen after scrolling to line n.
	top := func(n int) string {
		t.Helper()
		mm := m.(Model)
		if n > mm.maxScroll() {
			t.Fatalf("can't scroll to %d (max %d)", n, mm.maxScroll())
		}
		mm.scroll = n
		return strings.Split(mm.View(), "\n")[lipgloss.Height(mm.header())]
	}

	key('5')
	if hs := m.(Model).headings; len(hs) != 2 {
		t.Fatalf("headings = %v, want one per repo", hs)
	}
	second := m.(Model).headings[1]
	for n, want := range map[int]string{0: "● one", 5: "● one", second - 1: "● one", second: "● two", second + 3: "● two"} {
		if got := top(n); !strings.Contains(got, want) {
			t.Errorf("scrolled to %d: top line %q, want %q", n, got, want)
		}
	}

	// Side by side, the one heading row stays pinned.
	key('c')
	if hs := m.(Model).headings; len(hs) != 1 {
		t.Fatalf("side by side headings = %v, want one row", hs)
	}
	if got := top(8); !strings.Contains(got, "● one") || !strings.Contains(got, "● two") {
		t.Errorf("side by side, scrolled: top line %q", got)
	}

	// A single repo has no headings to pin.
	var single tea.Model = New(Options{Roots: []string{"/src/one"}, NoRemote: true})
	single, _ = single.Update(tea.WindowSizeMsg{Width: 160, Height: 20})
	single, _ = single.Update(localMsg{idx: 0, data: sampleLocal()})
	if hs := single.(Model).headings; len(hs) != 0 {
		t.Errorf("single repo headings = %v", hs)
	}
}

// Side by side, every repo's Authors table has the same columns, and repos stack
// rather than squeeze a table that can't fit.
func TestAuthorsColumnsMatch(t *testing.T) {
	wide := sampleLocal() // big numbers make this repo's table much wider
	for i := range wide.History.Authors {
		wide.History.Authors[i].Added *= 100000
		wide.History.Authors[i].Deleted *= 100000
	}
	for width := 100; width <= 220; width += 2 {
		var m tea.Model = New(Options{Roots: []string{"/src/small", "/src/wide"}, NoRemote: true})
		m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
		m, _ = m.Update(localMsg{idx: 0, data: sampleLocal()})
		m, _ = m.Update(localMsg{idx: 1, data: wide})
		for _, k := range "c2" {
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{k}})
		}
		mm := m.(Model)
		var headerRows []string
		for _, line := range mm.content {
			if strings.Contains(line, "│ # ") {
				headerRows = append(headerRows, line)
			}
		}
		if mm.columns() {
			if len(headerRows) != 1 {
				t.Fatalf("width %d: %d table header rows side by side", width, len(headerRows))
			}
			for _, col := range []string{"Commits", "Share", "Added", "Deleted", "First", "Last"} {
				if n := strings.Count(headerRows[0], col); n == 1 {
					t.Errorf("width %d: only one table has %s: %q", width, col, headerRows[0])
				}
			}
			if strings.Count(headerRows[0], "Author") != 2 {
				t.Errorf("width %d: a table was squeezed: %q", width, headerRows[0])
			}
		} else if !strings.Contains(mm.header(), "too narrow") {
			t.Errorf("width %d: stacked without saying why", width)
		}
	}
}

// Side by side, each repo's overview tiles line up with the other's, even
// when a label or value is wider in one repo than the other.
func TestOverviewTilesLineUp(t *testing.T) {
	var m tea.Model = New(Options{Roots: []string{"/src/app", "/src/api"}})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	for i := range 2 {
		m, _ = m.Update(localMsg{idx: i, data: sampleLocal()})
	}
	m, _ = m.Update(prMsg{idx: 0, data: &remote.PRStats{Open: 7, Merged: 312, MedianHoursToMerge: 30, Sample: 100}})
	m, _ = m.Update(prMsg{idx: 1, err: errors.New("no gh")}) // "PRs (from history)" is wider than "Merged PRs"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if !m.(Model).columns() {
		t.Fatal("repos didn't go side by side")
	}
	colW := m.(Model).columnWidth()
	corners := func(s string) []int {
		var at []int
		for i, r := range []rune(s) {
			if r == '╭' {
				at = append(at, i)
			}
		}
		return at
	}
	rows := 0
	for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		runes := []rune(line)
		if !strings.Contains(line, "╭") || len(runes) < 1+colW+columnGap {
			continue
		}
		rows++
		left, right := string(runes[1:1+colW]), string(runes[1+colW+columnGap:])
		if l, r := corners(left), corners(right); !slices.Equal(l, r) {
			t.Errorf("tile rows don't line up:\n%s\n%s", left, right)
		}
	}
	if rows < 2 {
		t.Errorf("expected tiles wrapping onto several rows, found %d", rows)
	}
}

// Pull Requests holds everything about PRs, works without GitHub, and its chart resizes.
func TestPullRequestsTab(t *testing.T) {
	var m tea.Model = New(Options{Roots: []string{"/src/repo"}, NoRemote: true})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	m, _ = m.Update(localMsg{idx: 0, data: sampleLocal()})
	key := func(r rune) { m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}) }
	body := func() string { return ansi.Strip(strings.Join(m.(Model).content, "\n")) }
	footer := func() string { return ansi.Strip(m.(Model).footer()) }

	key('5')
	for _, want := range []string{"PRs in history", "Mean PR size", "Median PR size", prChart + " (lines changed)", "PR size by author"} {
		if !strings.Contains(body(), want) {
			t.Errorf("Pull Requests is missing %q", want)
		}
	}
	if strings.Contains(body(), "Median time to merge") {
		t.Errorf("GitHub-only tiles show with --no-remote")
	}
	if f := footer(); !strings.Contains(f, "s sort PRs") || !strings.Contains(f, "+/- chart height") {
		t.Errorf("footer on Pull Requests: %s", f)
	}
	before := strings.Count(body(), "\n")
	key('+')
	if got := strings.Count(body(), "\n") - before; got != chartStep {
		t.Errorf("+ grew Pull Requests by %d lines, want %d", got, chartStep)
	}

	key('2')
	if strings.Contains(body(), "PR size by author") || strings.Contains(footer(), "s sort PRs") {
		t.Errorf("PR size by author is still on Authors")
	}
	key('3')
	if strings.Contains(body(), prChart) {
		t.Errorf("the PR size chart is still on Activity")
	}
}

// x adds the Email column to the Authors table and takes it away again, and
// only applies there. Once asked for, Email fits even where the table is narrow.
func TestDetailsToggle(t *testing.T) {
	for _, width := range []int{120, 80} {
		var m tea.Model = New(Options{Roots: []string{"/src/demo"}, NoRemote: true})
		m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
		m, _ = m.Update(localMsg{idx: 0, data: sampleLocal()})
		key := func(r rune) { m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}) }
		header := func() string {
			for _, line := range m.(Model).content {
				if strings.Contains(line, "│ # ") {
					return line
				}
			}
			return ""
		}

		key('2')
		if h := header(); strings.Contains(h, "Email") || !strings.Contains(h, "Author") {
			t.Errorf("width %d: email before x: %q", width, h)
		}
		if f := m.(Model).footer(); !strings.Contains(f, "x more details") {
			t.Errorf("width %d: footer = %q", width, f)
		}
		key('x')
		if h := header(); !strings.Contains(h, "Email") {
			t.Errorf("width %d: no email after x: %q", width, h)
		}
		if f := m.(Model).footer(); !strings.Contains(f, "x fewer details") {
			t.Errorf("width %d: footer = %q", width, f)
		}
		key('1')
		if strings.Contains(m.(Model).footer(), "details") {
			t.Errorf("x hint should only be on Authors")
		}
		key('2')
		key('x')
		if h := header(); strings.Contains(h, "Email") {
			t.Errorf("width %d: a second x should hide email: %q", width, h)
		}
	}
}
