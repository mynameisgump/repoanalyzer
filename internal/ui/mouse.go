package ui

import (
	"regexp"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// wheelStep is how many lines one notch of the mouse wheel scrolls.
const wheelStep = 3

// hotspot is a clickable stretch of screen line y, w cells wide from column x.
// Clicking it presses key; an empty key does nothing.
type hotspot struct {
	y, x, w int
	key     string
}

func (h hotspot) hit(x, y int) bool {
	return h.key != "" && y == h.y && x >= h.x && x < h.x+h.w
}

// expandNote matches listCut's notes, which e answers.
var expandNote = regexp.MustCompile(`(\+ [\d,]+ more · )?e to show (all|fewer)`)

// noteSpots finds the listCut notes on a body line shown at screen line y.
func noteSpots(line string, y int) []hotspot {
	plain := ansi.Strip(line)
	var spots []hotspot
	for _, loc := range expandNote.FindAllStringIndex(plain, -1) {
		x := ansi.StringWidth(plain[:loc[0]])
		spots = append(spots, hotspot{y: y, x: x, w: ansi.StringWidth(plain[loc[0]:loc[1]]), key: "e"})
	}
	return spots
}

// highlight redraws whichever of spots the pointer is over within s, a
// rendered line starting at screen column x, in hoverStyle.
func (m Model) highlight(s string, x int, spots ...hotspot) string {
	if !m.pointerSeen {
		return s
	}
	for _, h := range spots {
		if h.hit(m.pointerX, m.pointerY) {
			from, to := h.x-x, h.x-x+h.w
			return ansi.Cut(s, 0, from) + hoverStyle.Render(ansi.Strip(ansi.Cut(s, from, to))) + ansi.Cut(s, to, ansi.StringWidth(s))
		}
	}
	return s
}

// mouse scrolls for the wheel and returns the key a left click lands on, or "".
func (m *Model) mouse(msg tea.MouseMsg) string {
	if m.width < 64 || m.height < 12 { // View shows "window too small"
		return ""
	}
	switch msg.Button {
	case tea.MouseButtonWheelDown:
		m.scroll = min(m.scroll+wheelStep, m.maxScroll())
		return ""
	case tea.MouseButtonWheelUp:
		m.scroll = max(m.scroll-wheelStep, 0)
		return ""
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return ""
	}

	header, spots := m.headerLayout()
	headerH := lipgloss.Height(header)
	switch visible := m.visibleLines(); {
	case msg.Y < headerH:
	case msg.Y == m.height-1:
		_, spots = m.footerLayout()
	case msg.Y-headerH < len(visible):
		spots = noteSpots(visible[msg.Y-headerH], msg.Y)
	}
	for _, s := range spots {
		if s.hit(msg.X, msg.Y) {
			return s.key
		}
	}
	return ""
}
