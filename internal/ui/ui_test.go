package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ethancrann/repoanalyzer/internal/gitlog"
	"github.com/ethancrann/repoanalyzer/internal/loc"
	"github.com/ethancrann/repoanalyzer/internal/report"
	"github.com/ethancrann/repoanalyzer/internal/stats"
)

func sampleLocal() *report.Local {
	now := time.Now()
	var commits []gitlog.Commit
	for i := range 60 {
		commits = append(commits, gitlog.Commit{
			AuthorName: []string{"Ada", "Bob", "A very long author name indeed"}[i%3],
			When:       now.AddDate(0, 0, -i*5),
			Subject:    "change (#1)",
			Files:      []gitlog.FileChange{{Path: "some/really/long/path/to/a/file.go", Added: i, Deleted: 1}},
		})
	}
	h := stats.Compute(commits, now, nil)
	h.LinesOverTime = stats.LinesOverTime(commits, 25000, nil)
	h.PRSizes = stats.PRSizes(commits, nil, nil)
	return &report.Local{
		Repo: "demo", Branch: "main",
		LOC: loc.Result{Files: 3, Code: 100, TestFiles: 1, TestCode: 30,
			Languages: []loc.Language{{Name: "Go", Files: 2, Code: 90, TestFiles: 1, TestCode: 30}, {Name: "YAML", Files: 1, Code: 10}},
			Largest:   []loc.File{{Path: "main.go", Lines: 90}}},
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
	// Three charts each grow by one step.
	if got := lines(m) - before; got != 3*chartStep {
		t.Errorf("body grew by %d lines, want %d", got, 3*chartStep)
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
	mm.scroll = chartStarts(mm.body())[1] + 2 // a little way into the commits chart
	m = key(mm, '=')
	mm = m.(Model)
	if got, want := mm.scroll, chartStarts(mm.body())[1]+2; got != want {
		t.Errorf("scroll = %d, want %d", got, want)
	}
}
