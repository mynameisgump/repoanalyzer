package ui

import (
	"fmt"
	"strings"
)

// Activity line chart heights in rows; + and - step between them.
const (
	defaultChartH = 12
	minChartH     = 6
	chartStep     = 4
)

// Activity's line charts in page order; resizeCharts finds them by title.
var activityCharts = []string{"Total lines over time", "Commits per week", "Average PR size per month"}

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

// resizeCharts grows or shrinks the Activity charts by delta rows. The chart
// nearest the top of the screen keeps its title where it was, so the view
// doesn't jump as the charts above it change size.
func (m *Model) resizeCharts(delta int) {
	h := min(max(m.chartH()+delta, minChartH), m.maxChartH())
	if h == m.chartH() {
		return
	}
	anchor, offset := -1, 0
	for i, start := range chartStarts(m.body()) {
		if start >= 0 && start <= m.scroll {
			anchor, offset = i, m.scroll-start
		}
	}
	m.chartHeight = h
	if anchor >= 0 {
		if start := chartStarts(m.body())[anchor]; start >= 0 {
			m.scroll = start + offset
		}
	}
	m.scroll = min(m.scroll, m.maxScroll())
}

// chartStarts is the body line of each Activity chart's title, or -1 if it isn't shown.
func chartStarts(body string) []int {
	starts := []int{-1, -1, -1}
	for n, line := range strings.Split(body, "\n") {
		for i, title := range activityCharts {
			if starts[i] < 0 && strings.Contains(line, title) {
				starts[i] = n
			}
		}
	}
	return starts
}

// chartLabel shows a non-default chart height in the header while on Activity.
func (m Model) chartLabel() string {
	if m.tab != activityTab || m.chartH() == defaultChartH {
		return ""
	}
	return fmt.Sprintf("↕ %d rows", m.chartH())
}
