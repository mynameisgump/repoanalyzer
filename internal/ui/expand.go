package ui

import "fmt"

// showAll reports whether e has expanded the lists on the current tab.
func (m Model) showAll() bool {
	return m.expanded&(1<<m.tab) != 0
}

// expandable reports whether e changes anything on the current tab: some
// list there is long enough to be cut short.
func (m Model) expandable() bool {
	for _, spots := range m.spots {
		for _, s := range spots {
			if s.key == "e" {
				return true
			}
		}
	}
	return false
}

// listCut decides how many of a list's total rows to show: def normally, every
// row once the tab is expanded. note goes under the list and says how to switch,
// or is "" when the list is short enough that e changes nothing.
func listCut(total, def int, all bool) (n int, note string) {
	switch {
	case total <= def:
		return total, ""
	case all:
		return total, clickable("e", dimStyle.Render("e to show fewer"))
	}
	return def, clickable("e", dimStyle.Render(fmt.Sprintf("+ %s more · e to show all", num(total-def))))
}

// withNote appends a listCut note on its own line, if there is one.
func withNote(s, note string) string {
	if note == "" {
		return s
	}
	return s + "\n" + note
}
