package ui

import (
	"fmt"
	"slices"
	"strings"
)

// Line chart heights in rows, on Activity and Pull Requests; + and - step between them.
const (
	defaultChartH = 12
	minChartH     = 6
	chartStep     = 4
)

// Activity's, Authors' and Pull Requests' line charts in page order;
// resizeCharts finds them by title.
var (
	activityCharts = []string{"Total lines over time", "Commits per week", "Commits touching tests per month"}
	authorCharts   = []string{"Commits by author", "Active contributors per week"}
	prCharts       = []string{"Average PR size per month", "Commits via PR per month"}
	chartTitles    = slices.Concat(activityCharts, authorCharts, prCharts)
)

// hasCharts reports whether the current tab has line charts that + and - resize.
func (m Model) hasCharts() bool {
	return m.tab == authorsTab || m.tab == activityTab || m.tab == prTab
}

func (m Model) chartH() int {
	if m.chartHeight == 0 {
		return defaultChartH
	}
	return m.chartHeight
}

// maxChartH lets one chart and its title fill the screen, but never caps below the default.
func (m Model) maxChartH() int {
	return max(m.bodyHeight()-3, defaultChartH)
}

// resizeCharts grows or shrinks the line charts by delta rows. The chart
// nearest the top of the screen keeps its title where it was, so the view
// doesn't jump as the charts above it change size.
func (m *Model) resizeCharts(delta int) {
	h := min(max(m.chartH()+delta, minChartH), m.maxChartH())
	if h == m.chartH() {
		return
	}
	anchor, offset := -1, 0
	for i, start := range chartStarts(m.content) {
		if start >= 0 && start <= m.scroll {
			anchor, offset = i, m.scroll-start
		}
	}
	m.chartHeight = h
	m.render()
	if anchor >= 0 {
		if start := chartStarts(m.content)[anchor]; start >= 0 {
			m.scroll = min(start+offset, m.maxScroll())
		}
	}
}

// chartStarts is the line of each chart's title (see chartTitles), or -1 if it isn't shown.
func chartStarts(lines []string) []int {
	starts := make([]int, len(chartTitles))
	for i := range starts {
		starts[i] = -1
	}
	for n, line := range lines {
		// A line is the longest title it contains, in case one title holds another.
		best := -1
		for i, title := range chartTitles {
			if strings.Contains(line, title) && (best < 0 || len(title) > len(chartTitles[best])) {
				best = i
			}
		}
		if best >= 0 && starts[best] < 0 {
			starts[best] = n
		}
	}
	return starts
}

// chartLabel shows a non-default chart height in the header on a tab with charts.
func (m Model) chartLabel() string {
	if !m.hasCharts() || m.chartH() == defaultChartH {
		return ""
	}
	return fmt.Sprintf("↕ %d rows", m.chartH())
}
