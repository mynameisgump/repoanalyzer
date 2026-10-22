package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// wheelStep is how many lines one notch of the mouse wheel scrolls.
const wheelStep = 3

// hotspot is a clickable stretch of screen line y, w cells wide from column x.
// Clicking it presses key, or does a click-only action like sortClick; an
// empty key does nothing.
type hotspot struct {
	y, x, w int
	key     string
}

func (h hotspot) hit(x, y int) bool {
	return h.key != "" && y == h.y && x >= h.x && x < h.x+h.w
}

// sortClick followed by a prSortKey digit sorts PR size by author by that column.
const sortClick = "sort:"

// The body is drawn once and then scrolled, so its clickable parts are marked
// in the text as it's drawn, with zero-width escape sequences that layout
// passes over. render takes the marks out again and keeps where they were.
const (
	markPrefix = "\x1b]7777"
	markOpen   = markPrefix + ";" // then the key and markEnd
	markEnd    = "\a"
	markClose  = markPrefix + markEnd
)

// clickable marks s in body text so a click on it presses key.
func clickable(key, s string) string {
	return markOpen + key + markEnd + s + markClose
}

// takeSpots removes the clickable marks from a line of content, returning
// the line and where the marked parts are (y is left for bodySpots).
func takeSpots(line string) (string, []hotspot) {
	var spots []hotspot
	for {
		i := strings.Index(line, markOpen)
		if i < 0 {
			break
		}
		rest := line[i+len(markOpen):]
		k := strings.Index(rest, markEnd)
		if k < 0 {
			break
		}
		key := rest[:k]
		line = line[:i] + rest[k+len(markEnd):]
		// A mark cut off by wrapping runs to the end of the line.
		j := strings.Index(line[i:], markClose)
		if j < 0 {
			j = len(line) - i
		}
		spots = append(spots, hotspot{x: ansi.StringWidth(line[:i]), w: ansi.StringWidth(line[i : i+j]), key: key})
	}
	return strings.ReplaceAll(line, markClose, ""), spots
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
	footer, footerSpots := m.footerLayout()
	switch {
	case msg.Y < lipgloss.Height(header):
	case msg.Y >= m.height-lipgloss.Height(footer):
		spots = footerSpots
	default:
		spots = m.bodySpots(msg.Y)
	}
	for _, s := range spots {
		if s.hit(msg.X, msg.Y) {
			return s.key
		}
	}
	return ""
}
