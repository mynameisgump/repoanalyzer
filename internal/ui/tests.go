package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// fileFilter limits the Files tab's lists to test or non-test files; f cycles it.
type fileFilter int

const (
	allFiles fileFilter = iota
	nonTestFiles
	testFilesOnly
	fileFilters // number of filters
)

// keep reports whether a file passes the filter.
func (f fileFilter) keep(test bool) bool {
	switch f {
	case nonTestFiles:
		return !test
	case testFilesOnly:
		return test
	}
	return true
}

func (f fileFilter) label() string {
	return [...]string{"all files", "non-test files", "test files only"}[f]
}

// filterLabel shows an active file filter in the header while on Files.
func (m Model) filterLabel() string {
	if m.tab != filesTab || m.fileFilter == allFiles {
		return ""
	}
	return "⚑ " + m.fileFilter.label()
}

// testTag marks a test file in a list.
func testTag(test bool) string {
	if !test {
		return ""
	}
	return "  " + testStyle.Render("test")
}

// percent formats a 0–100 axis value.
func percent(v float64) string {
	return fmt.Sprintf("%.0f%%", v)
}

// testChart plots the monthly share of commits that changed a test file, one line per repo.
func testChart(ss []series, summaries []string, w, h int, from, to time.Time) string {
	found := false
	for _, s := range ss {
		for _, v := range s.vals {
			found = found || v > 0
		}
	}
	if !found {
		return dimStyle.Render("No commits changed a test file.")
	}
	return timeChart(ss, w, h, percent, chartBounds{from: from, to: to}) + "\n" + strings.Join(summaries, "\n")
}

// testSummary is an overall test share for a repo, wrapped to w.
func (m Model) testSummary(r *repoState, w int) string {
	t := r.hist.Tests
	text := fmt.Sprintf("%s of %s commits (%s) changed tests · %s of %s lines added (%s) went to tests",
		num(t.TestCommits), num(t.Commits), pct(t.TestCommits, t.Commits),
		num(t.TestAdded), num(t.Added), pct(t.TestAdded, t.Added))
	if m.comparing() {
		text = lipgloss.NewStyle().Foreground(r.color).Render("● "+r.name) + "  " + text
	}
	return lipgloss.NewStyle().Width(w).Render(text)
}
