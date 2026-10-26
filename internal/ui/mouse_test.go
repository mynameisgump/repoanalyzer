package ui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/mynameisgump/repoanalyzer/internal/loc"
	"github.com/mynameisgump/repoanalyzer/internal/stats"
)

// Clicks land on what the screen shows there, found by looking at the rendered view.
func TestMouse(t *testing.T) {
	var m tea.Model = New(Options{Roots: []string{"/src/one", "/src/two"}, NoRemote: true})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	for i := range 2 {
		local := sampleLocal()
		for j := range 30 {
			local.LOC.Languages = append(local.LOC.Languages, loc.Language{Name: "Lang" + string(rune('A'+j)), Code: 1})
		}
		m, _ = m.Update(localMsg{idx: i, data: local})
	}
	// find is the screen cell where text starts, searching from the bottom when last.
	find := func(text string, last bool) (x, y int) {
		t.Helper()
		lines := strings.Split(m.View(), "\n")
		for n := range lines {
			if last {
				n = len(lines) - 1 - n
			}
			plain := ansi.Strip(lines[n])
			if i := strings.Index(plain, text); i >= 0 {
				return ansi.StringWidth(plain[:i]), n
			}
		}
		t.Fatalf("%q isn't on screen:\n%s", text, ansi.Strip(m.View()))
		return 0, 0
	}
	click := func(x, y int) {
		m, _ = m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	}

	for i, name := range tabNames {
		x, y := find(name, false)
		click(x+1, y)
		if got := m.(Model).tab; got != i {
			t.Fatalf("clicked %s, tab = %d", name, got)
		}
	}
	click(find("Overview", false))

	wheel := func(b tea.MouseButton) {
		m, _ = m.Update(tea.MouseMsg{Button: b, Action: tea.MouseActionPress})
	}
	wheel(tea.MouseButtonWheelDown)
	wheel(tea.MouseButtonWheelDown)
	if got := m.(Model).scroll; got != 2*wheelStep {
		t.Errorf("scroll after two wheel notches = %d", got)
	}
	wheel(tea.MouseButtonWheelUp)
	if got := m.(Model).scroll; got != wheelStep {
		t.Errorf("scroll after wheel up = %d", got)
	}

	x, y := find("e to show all", false)
	click(x+2, y)
	if !m.(Model).showAll() {
		t.Errorf("clicking the note didn't expand the list")
	}

	x, y = find("t window", true)
	click(x+4, y)
	if got := m.(Model).window; got != windowYear {
		t.Errorf("clicking the window hint: window = %v", got)
	}
	// The header now shows the window; clicking it cycles on.
	x, y = find("◷ last year", false)
	click(x+3, y)
	if got := m.(Model).window; got != windowHalf {
		t.Errorf("clicking the window setting: window = %v", got)
	}

	click(find("c columns", true))
	if !m.(Model).sideBySide {
		t.Errorf("clicking the columns hint didn't turn columns on")
	}

	click(find("Activity", false))
	before := m.(Model).chartH()
	click(find("+/- chart height", true))
	if got := m.(Model).chartH(); got != before+chartStep {
		t.Errorf("clicking + : chart height = %d, want %d", got, before+chartStep)
	}
	x, y = find("+/- chart height", true)
	click(x+2, y)
	if got := m.(Model).chartH(); got != before {
		t.Errorf("clicking - : chart height = %d, want %d", got, before)
	}
	// Between the + and - is the slash, which does nothing.
	click(x+1, y)
	if got := m.(Model).chartH(); got != before {
		t.Errorf("clicking / changed the chart height to %d", got)
	}

	// Clicks on plain text do nothing.
	click(1, 0)
	if got := m.(Model).tab; got != activityTab {
		t.Errorf("clicking the title switched to tab %d", got)
	}
}

// Hovering over something clickable highlights exactly that, and nothing else lights up.
func TestHover(t *testing.T) {
	defer lipgloss.SetColorProfile(lipgloss.ColorProfile())
	lipgloss.SetColorProfile(termenv.TrueColor)

	var m tea.Model = New(Options{Roots: []string{"/src/one", "/src/two"}, NoRemote: true})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	for i := range 2 {
		local := sampleLocal()
		for j := range 30 {
			local.LOC.Languages = append(local.LOC.Languages, loc.Language{Name: "Lang" + string(rune('A'+j)), Code: 1})
		}
		m, _ = m.Update(localMsg{idx: i, data: local})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}) // puts the window in the header
	hoverOn, _, _ := strings.Cut(hoverStyle.Render("x"), "x")
	if hoverOn == "" {
		t.Fatal("hoverStyle renders without color")
	}
	find := func(text string, last bool) (x, y int) {
		t.Helper()
		lines := strings.Split(m.View(), "\n")
		for n := range lines {
			if last {
				n = len(lines) - 1 - n
			}
			plain := ansi.Strip(lines[n])
			if i := strings.Index(plain, text); i >= 0 {
				return ansi.StringWidth(plain[:i]), n
			}
		}
		t.Fatalf("%q isn't on screen:\n%s", text, ansi.Strip(m.View()))
		return 0, 0
	}
	hover := func(x, y int) string {
		m, _ = m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion})
		return m.View()
	}

	cases := []struct {
		name, text, lit string
		last            bool
		dx              int
	}{
		{"a tab", "Authors", " 2 Authors ", false, 0},
		{"a footer hint", "t window", "t window", true, 2},
		{"a header setting", "◷ last year", "◷ " + m.(Model).windowLabel(), false, 4},
		{"a more note", "e to show all", "+ 22 more · e to show all", false, 0},
	}
	for _, c := range cases {
		x, y := find(c.text, c.last)
		if v := hover(x+c.dx, y); !strings.Contains(v, hoverStyle.Render(c.lit)) {
			t.Errorf("hovering %s didn't highlight %q", c.name, c.lit)
		} else if strings.Count(v, hoverOn) != 1 {
			t.Errorf("hovering %s highlighted %d things", c.name, strings.Count(v, hoverOn))
		}
	}

	// Only the + of "+/- chart height" lights up over it.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	x, y := find("+/- chart height", true)
	if v := hover(x, y); !strings.Contains(v, hoverStyle.Render("+")) || strings.Count(v, hoverOn) != 1 {
		t.Errorf("hovering + didn't highlight just the +")
	}

	// The current tab, plain text and empty space don't light up.
	ax, ay := find("Activity", false)
	for _, at := range [][2]int{{ax, ay}, {1, 0}, {80, 15}} {
		if v := hover(at[0], at[1]); strings.Contains(v, hoverOn) {
			t.Errorf("hovering %v highlighted something", at)
		}
	}
}

// Clicking a PR size by author column title sorts by it, stacked or side by side.
func TestClickPRSort(t *testing.T) {
	defer lipgloss.SetColorProfile(lipgloss.ColorProfile())
	lipgloss.SetColorProfile(termenv.TrueColor)
	hoverOn, _, _ := strings.Cut(hoverStyle.Render("x"), "x")

	for _, columns := range []bool{false, true} {
		var m tea.Model = New(Options{Roots: []string{"/src/one", "/src/two"}, NoRemote: true})
		m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 120})
		for i := range 2 {
			local := sampleLocal()
			local.History.PRSizes.ByAuthor = []stats.AuthorPRs{
				{Name: "Avg", PRs: 2, MeanLines: 900, MedianLines: 10},
				{Name: "Busy", PRs: 40, MeanLines: 100, MedianLines: 60},
			}
			m, _ = m.Update(localMsg{idx: i, data: local})
		}
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
		if columns {
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
			if !m.(Model).columns() {
				t.Fatal("repos didn't go side by side")
			}
		}
		// at is where the nth title on the PR size header row is: repo two's is
		// the second one on the line when side by side, else on a later line.
		at := func(title string, repo int) (x, y int) {
			t.Helper()
			seen := 0
			for n, line := range strings.Split(ansi.Strip(m.View()), "\n") {
				if !strings.Contains(line, "avg lines") {
					continue
				}
				for i := 0; ; {
					j := strings.Index(line[i:], title)
					if j < 0 {
						break
					}
					if seen == repo {
						return ansi.StringWidth(line[:i+j]), n
					}
					seen++
					i += j + len(title)
				}
			}
			t.Fatalf("no %q title for repo %d:\n%s", title, repo, ansi.Strip(m.View()))
			return 0, 0
		}
		mouse := func(x, y int, b tea.MouseButton, a tea.MouseAction) string {
			m, _ = m.Update(tea.MouseMsg{X: x, Y: y, Button: b, Action: a})
			return m.View()
		}

		// Sorted by average to start with, so that title isn't clickable.
		x, y := at("avg lines", 0)
		if v := mouse(x+1, y, tea.MouseButtonNone, tea.MouseActionMotion); strings.Contains(v, hoverOn) {
			t.Errorf("columns=%v: the sorted column's title lights up", columns)
		}
		for _, c := range []struct {
			title string
			repo  int
			want  prSortKey
		}{{"PRs", 0, sortByPRs}, {"median lines", 1, sortByMedian}, {"avg lines", 1, sortByMean}} {
			x, y := at(c.title, c.repo)
			if v := mouse(x, y, tea.MouseButtonNone, tea.MouseActionMotion); !strings.Contains(v, hoverStyle.Render(c.title)) {
				t.Errorf("columns=%v: hovering %q doesn't light it up", columns, c.title)
			}
			v := mouse(x, y, tea.MouseButtonLeft, tea.MouseActionPress)
			if got := m.(Model).prSort; got != c.want {
				t.Errorf("columns=%v: clicking %q sorted by %d", columns, c.title, got)
			}
			if !strings.Contains(ansi.Strip(v), c.title+" ▼") {
				t.Errorf("columns=%v: %q isn't marked sorted after clicking it", columns, c.title)
			}
		}
	}
}

// The marks for clickable body text never reach the screen.
func TestNoMarksOnScreen(t *testing.T) {
	var m tea.Model = New(Options{Roots: []string{"/src/one", "/src/two"}, NoRemote: true})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	for i := range 2 {
		m, _ = m.Update(localMsg{idx: i, data: sampleLocal()})
	}
	for _, k := range "c12345e12345" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{k}})
		if strings.Contains(m.View(), markPrefix) {
			t.Fatalf("a click mark is on screen after %c", k)
		}
	}
}

// The footer lists every control that does something now, wrapping instead of
// leaving any out, and its hints stay clickable on every line.
func TestFooterControls(t *testing.T) {
	var m tea.Model = New(Options{Roots: []string{"/src/one", "/src/two"}, NoRemote: true})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 64, Height: 30})
	for i := range 2 {
		local := sampleLocal()
		if i == 0 {
			for j := range 30 {
				local.LOC.Languages = append(local.LOC.Languages, loc.Language{Name: "Lang" + string(rune('A'+j)), Code: 1})
			}
		}
		m, _ = m.Update(localMsg{idx: i, data: local})
	}
	footer := func() string { return ansi.Strip(m.(Model).footer()) }
	for _, want := range []string{"←/→ tabs", "↑/↓ scroll", "pgup/pgdn page", "t window", "o overlap", "c columns", "e show all", "q quit"} {
		if !strings.Contains(footer(), want) {
			t.Errorf("footer is missing %q:\n%s", want, footer())
		}
	}
	if n := strings.Count(footer(), "\n") + 1; n < 2 {
		t.Fatalf("footer didn't wrap at 64 wide:\n%s", footer())
	}

	// Clicking a hint on the second line presses its key.
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	last := lines[len(lines)-1]
	x := strings.Index(last, "c columns")
	if x < 0 {
		t.Fatalf("c columns isn't on the last line: %q", last)
	}
	m, _ = m.Update(tea.MouseMsg{X: ansi.StringWidth(last[:x]), Y: len(lines) - 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !m.(Model).sideBySide {
		t.Errorf("clicking c columns on the footer's second line didn't press c")
	}

	// e's hint says what it does next, and goes away where e does nothing.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if !strings.Contains(footer(), "e show fewer") {
		t.Errorf("after e, footer doesn't offer show fewer:\n%s", footer())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	if strings.Contains(footer(), "e show") {
		t.Errorf("footer offers e on a tab with no long lists:\n%s", footer())
	}
}

// Clicking names in the Commits by author legend limits the chart to those
// authors, and clicking one again puts the others back; hovering a name draws
// that author over the rest in grey.
func TestClickFocusAuthors(t *testing.T) {
	defer lipgloss.SetColorProfile(lipgloss.ColorProfile())
	lipgloss.SetColorProfile(termenv.TrueColor)
	color := func(i int) string {
		on, _, _ := strings.Cut(lipgloss.NewStyle().Foreground(palette[i]).Render("x"), "x")
		return on
	}
	grey, _, _ := strings.Cut(dimStyle.Render("x"), "x")

	var m tea.Model = New(Options{Roots: []string{"/src/repo"}, NoRemote: true})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	m, _ = m.Update(localMsg{idx: 0, data: sampleLocal()})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	at := func(text string) (x, y int) {
		t.Helper()
		for n, line := range strings.Split(ansi.Strip(m.View()), "\n") {
			if i := strings.Index(line, text); i >= 0 {
				return ansi.StringWidth(line[:i]), n
			}
		}
		t.Fatalf("%q isn't on screen:\n%s", text, ansi.Strip(m.View()))
		return 0, 0
	}
	click := func(name string) {
		t.Helper()
		x, y := at(" " + name + "   ")
		m, _ = m.Update(tea.MouseMsg{X: x + 1, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	}
	// chart is the plot itself, without the legend above it.
	chart := func() string {
		body := m.(Model).body()
		_, plot, _ := strings.Cut(body[strings.Index(body, "Commits by author"):], "│")
		plot, _, _ = strings.Cut(plot, "Active contributors")
		return plot
	}

	// Authors by rank: the long name, Ada, Bob.
	click("Bob")
	if got := m.(Model).repos[0].focused; !slices.Equal(got, []string{"Bob"}) {
		t.Errorf("clicking Bob focused %v", got)
	}
	if c := chart(); !strings.Contains(c, color(2)) || strings.Contains(c, color(0)) || strings.Contains(c, color(1)) {
		t.Errorf("with Bob picked out, the chart still has the others")
	}
	if !strings.Contains(ansi.Strip(m.View()), "╌╌ Ada") {
		t.Errorf("Ada isn't shown as off the chart")
	}
	click("Ada")
	if got := m.(Model).focusLabel(); got != "◎ Bob, Ada" {
		t.Errorf("header says %q after adding Ada", got)
	}
	click("Bob")
	click("Ada")
	if got := m.(Model).repos[0].focused; len(got) != 0 {
		t.Errorf("unclicking everyone left %v", got)
	}

	// Hovering Ada greys everyone else.
	x, y := at("━━ Ada")
	m, _ = m.Update(tea.MouseMsg{X: x + 1, Y: y, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion})
	if got := m.(Model).hover; got != (authorRef{repo: "repo", name: "Ada"}) {
		t.Fatalf("hover = %+v", got)
	}
	if c := chart(); !strings.Contains(c, color(1)) || !strings.Contains(c, grey) || strings.Contains(c, color(0)) || strings.Contains(c, color(2)) {
		t.Errorf("hovering Ada didn't grey the others")
	}
	m, _ = m.Update(tea.MouseMsg{X: 100, Y: 45, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion})
	if c := chart(); m.(Model).hover != (authorRef{}) || !strings.Contains(c, color(0)) {
		t.Errorf("moving off Ada didn't bring the colors back")
	}
}
