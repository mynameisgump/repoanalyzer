package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/mynameisgump/repoanalyzer/internal/loc"
)

// Clicks land on what the screen shows there, found by looking at the rendered view.
func TestMouse(t *testing.T) {
	var m tea.Model = New(Options{Roots: []string{"/src/one", "/src/two"}, NoRemote: true})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
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
