package ui

import "fmt"

// showAll reports whether e has expanded the lists on the current tab.
func (m Model) showAll() bool {
	return m.expanded&(1<<m.tab) != 0
}

// listCut decides how many of a list's total rows to show: def normally, every
// row once the tab is expanded. note goes under the list and says how to switch,
// or is "" when the list is short enough that e changes nothing.
func listCut(total, def int, all bool) (n int, note string) {
	switch {
	case total <= def:
		return total, ""
	case all:
		return total, dimStyle.Render("e to show fewer")
	}
	return def, dimStyle.Render(fmt.Sprintf("+ %s more · e to show all", num(total-def)))
}

// withNote appends a listCut note on its own line, if there is one.
func withNote(s, note string) string {
	if note == "" {
		return s
	}
	return s + "\n" + note
}
