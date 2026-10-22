package ui

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/NimbleMarkets/ntcharts/linechart/timeserieslinechart"
	"github.com/NimbleMarkets/ntcharts/sparkline"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"

	"github.com/mynameisgump/repoanalyzer/internal/loc"
	"github.com/mynameisgump/repoanalyzer/internal/stats"
)

func section(title string) string {
	return sectionStyle.Render(title)
}

func (m Model) overview(w int) string {
	if !m.comparing() {
		return m.each(w, oneSection(m.overviewRepo))
	}
	return m.comparison(w) + "\n" + section("Languages (code lines, excl. blank)"+m.treeNote()) + "\n" +
		m.each(w, func(r *repoState, w int) []string { return []string{languages(r, w, m.showAll())} })
}

func (m Model) overviewRepo(r *repoState, w int) string {
	l, h := r.local.LOC, r.hist
	// Age is the repo's, not the time window's.
	age := "—"
	if r.local.History.Commits > 0 {
		age = humanSpan(time.Since(r.local.History.First))
	}
	prLabel := "Merged PRs"
	if r.prs == nil && r.prDone || m.opts.NoRemote {
		prLabel = "PRs (from history)"
	}
	tiles := flow([]string{
		tile("Lines of code", num(l.Code)),
		tile("Files", num(l.Files)),
		tile("Commits", num(h.Commits)),
		tile("Contributors", num(len(h.Authors))),
		tile("Active (90d)", num(h.ActiveAuthors90d)),
		tile("Repo age", age),
		tile(prLabel, m.prValue(r)),
	}, w)

	out := tiles + "\n" + section("Languages (code lines, excl. blank)"+m.treeNote()) + "\n" + languages(r, w, m.showAll())
	if len(h.Weekly) > 1 {
		out += "\n" + section("Commits per week") + "\n" + weeklySparkline(h.Weekly, w, r.color)
	}
	return out
}

// prValue is the merged-PR count, an estimate from history, or "…" while loading.
func (m Model) prValue(r *repoState) string {
	switch {
	case r.prs != nil:
		return num(r.prs.Merged)
	case m.opts.NoRemote || r.prDone:
		return "~" + num(r.hist.PRsInHistory)
	}
	return "…"
}

// comparison is a metric-by-repo table for the overview when comparing.
func (m Model) comparison(w int) string {
	metrics := []struct {
		label string
		value func(r *repoState) string
	}{
		{"Lines of code", func(r *repoState) string { return num(r.local.LOC.Code) }},
		{"Files", func(r *repoState) string { return num(r.local.LOC.Files) }},
		{"Commits", func(r *repoState) string { return num(r.hist.Commits) }},
		{"Contributors", func(r *repoState) string { return num(len(r.hist.Authors)) }},
		{"Active (90d)", func(r *repoState) string { return num(r.hist.ActiveAuthors90d) }},
		{"Repo age", func(r *repoState) string {
			if r.local.History.Commits == 0 {
				return "—"
			}
			return humanSpan(time.Since(r.local.History.First))
		}},
		{"Merged PRs", m.prValue},
		{"Top language", func(r *repoState) string {
			if len(r.local.LOC.Languages) == 0 {
				return "—"
			}
			top := r.local.LOC.Languages[0]
			return top.Name + " " + pct(top.Code, r.local.LOC.Code)
		}},
	}

	headers := []string{""}
	for _, r := range m.repos {
		headers = append(headers, truncate(r.name, 24))
	}
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(dimStyle).
		Headers(headers...).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().Padding(0, 1)
			switch {
			case col == 0:
				return s.Foreground(subtle)
			case row == table.HeaderRow:
				return s.Bold(true).Foreground(m.repos[col-1].color).Align(lipgloss.Right)
			}
			return s.Align(lipgloss.Right)
		})
	for _, mt := range metrics {
		row := []string{mt.label}
		for i := range m.repos {
			r := &m.repos[i]
			switch {
			case r.localErr != nil:
				row = append(row, "error")
			case r.local == nil:
				row = append(row, "…")
			default:
				row = append(row, mt.value(r))
			}
		}
		t.Row(row...)
	}
	out := t.Render()
	if lipgloss.Width(out) > w {
		out = t.Width(w).Render()
	}
	if m.opts.NoRemote || m.anyPRFallback() {
		out += "\n" + dimStyle.Render("~ = estimated from commit messages")
	}
	return out
}

func (m Model) anyPRFallback() bool {
	for _, r := range m.repos {
		if r.prDone && r.prs == nil {
			return true
		}
	}
	return false
}

func languages(r *repoState, w int, all bool) string {
	l := r.local.LOC
	n, note := listCut(len(l.Languages), 10, all)
	var rows []barRow
	for _, lang := range l.Languages[:n] {
		rows = append(rows, barRow{
			label: lang.Name,
			value: float64(lang.Code),
			text:  fmt.Sprintf("%9s  %5s", num(lang.Code), pct(lang.Code, l.Code)),
		})
	}
	return withNote(barChart(rows, w), note)
}

func weeklySparkline(weeks []stats.Bucket, w int, color lipgloss.TerminalColor) string {
	// One column per week; show the most recent weeks that fit.
	if len(weeks) > w {
		weeks = weeks[len(weeks)-w:]
	}
	vals := make([]float64, len(weeks))
	for i, b := range weeks {
		vals[i] = float64(b.Commits)
	}
	sl := sparkline.New(len(vals), 4, sparkline.WithStyle(lipgloss.NewStyle().Foreground(color)))
	sl.PushAll(vals)
	sl.Draw()
	from := weeks[0].Start.Format("Jan 2006")
	to := weeks[len(weeks)-1].Start.Format("Jan 2006")
	gap := max(len(vals)-len(from)-len(to), 1)
	return sl.View() + "\n" + dimStyle.Render(from+strings.Repeat(" ", gap)+to)
}

func (m Model) authors(w int) string {
	limit, windowed, all := m.authorsLimit(), m.windowed(), m.showAll()
	colW := w
	if m.columns() {
		colW = m.columnWidth()
	}
	headers, _ := m.authorColumns(colW)
	return m.each(w, func(r *repoState, w int) []string {
		return authorsRepo(r, w, limit, windowed, all, m.prSort, headers)
	})
}

// authorsLimit is how many authors the table shows before e expands it.
func (m Model) authorsLimit() int {
	if m.comparing() {
		return 15 // keep both repos within a reasonable scroll
	}
	return 50
}

// authorColumns picks the Authors table's columns for tables w wide. Every repo
// gets the same ones: Share, then Last, are dropped until the widest repo's table
// fits. fits is false when even the fewest columns are too wide, and the
// tables will be squeezed, wrapping names and numbers.
func (m Model) authorColumns(w int) (headers []string, fits bool) {
	headers = []string{"#", "Author", "Commits", "Share", "Added", "Deleted", "Last"}
	if w >= 110 { // room for the share bar and the first commit's date
		headers = []string{"#", "Author", "Commits", "Share", "Added", "Deleted", "First", "Last"}
	}
	limit, all := m.authorsLimit(), m.showAll()
	fit := func(headers []string) bool {
		for i := range m.repos {
			r := &m.repos[i]
			if r.local != nil && len(r.hist.Authors) > 0 && lipgloss.Width(authorsTable(r, w, limit, all, headers).Render()) > w {
				return false
			}
		}
		return true
	}
	for _, drop := range []string{"", "Share", "Last"} {
		if drop != "" {
			headers = slices.DeleteFunc(slices.Clone(headers), func(h string) bool { return h == drop })
		}
		if fit(headers) {
			return headers, true
		}
	}
	return headers, false
}

func authorsRepo(r *repoState, w, limit int, windowed, all bool, prSort prSortKey, headers []string) []string {
	h := r.hist
	if len(h.Authors) == 0 {
		if windowed {
			return []string{dimStyle.Render("No commits in this time window.")}
		}
		return []string{dimStyle.Render("No commits yet.")}
	}
	recent := "the last 90 days"
	if windowed {
		recent = "the window's last 90 days"
	}
	summary := fmt.Sprintf("%s contributors · %s active in %s",
		boldStyle.Render(num(len(h.Authors))), boldStyle.Render(num(h.ActiveAuthors90d)), recent)

	_, more := listCut(len(h.Authors), limit, all)
	t := authorsTable(r, w, limit, all, headers)
	rendered := t.Render()
	if lipgloss.Width(rendered) > w {
		rendered = t.Width(w).Render() // let lipgloss squeeze the columns
	}
	sections := []string{section("Contributors by commits") + "\n" + withNote(summary+"\n"+rendered, more)}
	if len(h.PRSizes.ByAuthor) > 0 {
		sections = append(sections, section("PR size by author (lines changed per PR)")+"\n"+
			prSizesByAuthor(h.PRSizes, w, min(limit, 10), all, prSort))
	}
	return sections
}

// authorsTable is a repo's contributors table with the given columns (see authorColumns).
func authorsTable(r *repoState, w, limit int, all bool, headers []string) *table.Table {
	h := r.hist
	n, _ := listCut(len(h.Authors), limit, all)
	authors := h.Authors[:n]
	wide := slices.Contains(headers, "First")
	nameW := min(28, max(w/4, 12))
	topCommits := float64(authors[0].Commits)
	cell := func(i int, a stats.Author, col string) string {
		switch col {
		case "#":
			return fmt.Sprint(i + 1)
		case "Author":
			return truncate(a.Name, nameW)
		case "Commits":
			return num(a.Commits)
		case "Share":
			if wide {
				return hbar(float64(a.Commits), topCommits, 10) + " " + pct(a.Commits, h.Commits)
			}
			return pct(a.Commits, h.Commits)
		case "Added":
			return "+" + num(a.Added)
		case "Deleted":
			return "-" + num(a.Deleted)
		case "First":
			return a.First.Format("2006-01-02")
		}
		return ago(a.Last)
	}
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(dimStyle).
		Headers(headers...).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().Padding(0, 1)
			if row == table.HeaderRow {
				return s.Bold(true).Foreground(r.color)
			}
			switch headers[col] {
			case "#":
				s = s.Align(lipgloss.Right).Foreground(subtle)
			case "Commits":
				s = s.Align(lipgloss.Right)
			case "Added":
				s = s.Align(lipgloss.Right).Foreground(green)
			case "Deleted":
				s = s.Align(lipgloss.Right).Foreground(red)
			case "First", "Last":
				s = s.Foreground(subtle)
			}
			return s
		})
	for i, a := range authors {
		var row []string
		for _, col := range headers {
			row = append(row, cell(i, a, col))
		}
		t.Row(row...)
	}
	return t
}

// prSortKey picks the column PR size by author is ordered by, largest first.
type prSortKey int

const (
	sortByMean prSortKey = iota
	sortByMedian
	sortByPRs
	prSortKeys // number of keys
)

// prSizesByAuthor charts each author's average PR size, median PR size and PR
// count as three bar columns, each scaled on its own, ordered by sortBy.
func prSizesByAuthor(ps stats.PRSizeStats, w, limit int, all bool, sortBy prSortKey) string {
	// Sort a copy: ByAuthor itself stays most-PRs-first for --json.
	authors := slices.Clone(ps.ByAuthor)
	sortAuthorPRs(authors, sortBy)
	n, more := listCut(len(authors), limit, all)
	authors = authors[:n]

	type column struct {
		title string
		color lipgloss.TerminalColor
		value func(stats.AuthorPRs) float64
		valW  int
		peak  float64
	}
	cols := []*column{
		{title: "avg lines", color: palette[0], value: func(a stats.AuthorPRs) float64 { return math.Round(a.MeanLines) }},
		{title: "median lines", color: palette[1], value: func(a stats.AuthorPRs) float64 { return float64(a.MedianLines) }},
		{title: "PRs", color: palette[3], value: func(a stats.AuthorPRs) float64 { return float64(a.PRs) }},
	}
	labelW := 0
	for _, a := range authors {
		labelW = max(labelW, min(lipgloss.Width(a.Name), 24))
		for _, c := range cols {
			c.valW = max(c.valW, len(num(int(c.value(a)))))
			c.peak = math.Max(c.peak, c.value(a))
		}
	}
	// Each column is a bar, a space and its right-aligned value; columns are 3 apart.
	const gap = 3
	fixed := labelW + 2
	for _, c := range cols {
		fixed += 1 + c.valW
	}
	fixed += gap * (len(cols) - 1)
	barW := (w - fixed) / len(cols)
	if barW < 3 {
		barW = 0 // too narrow for bars; the numbers alone still line up
	}

	var b strings.Builder
	b.WriteString(strings.Repeat(" ", labelW+2))
	for i, c := range cols {
		colW := barW + 1 + c.valW
		style := lipgloss.NewStyle().Foreground(c.color)
		text := truncate(c.title, colW)
		if prSortKey(i) == sortBy { // shorten the title, never the marker
			style = style.Bold(true)
			text = truncate(c.title, max(colW-2, 1)) + " ▼"
		}
		title := style.Render(text)
		if barW == 0 {
			title = style.Width(colW).Align(lipgloss.Right).Render(text)
		}
		if prSortKey(i) != sortBy {
			title = clickable(sortClick+strconv.Itoa(i), title)
		}
		if i < len(cols)-1 {
			title = lipgloss.NewStyle().Width(colW + gap).Render(title)
		}
		b.WriteString(title)
	}
	for _, a := range authors {
		b.WriteString("\n" + lipgloss.NewStyle().Width(labelW+2).Render(truncate(a.Name, labelW)))
		for i, c := range cols {
			v := c.value(a)
			if barW > 0 {
				b.WriteString(lipgloss.NewStyle().Foreground(c.color).Width(barW).Render(hbar(v, c.peak, barW)) + " ")
			}
			b.WriteString(fmt.Sprintf("%*s", c.valW, num(int(v))))
			if i < len(cols)-1 {
				b.WriteString(strings.Repeat(" ", gap))
			}
		}
	}
	return withNote(b.String(), more)
}

// sortAuthorPRs orders authors by the chosen column, largest first. Ties go to
// the average, then the PR count, then the name, so the order is stable.
func sortAuthorPRs(authors []stats.AuthorPRs, by prSortKey) {
	keys := func(a stats.AuthorPRs) []float64 {
		switch by {
		case sortByMedian:
			return []float64{float64(a.MedianLines), a.MeanLines, float64(a.PRs)}
		case sortByPRs:
			return []float64{float64(a.PRs), a.MeanLines}
		}
		return []float64{a.MeanLines, float64(a.PRs)}
	}
	sort.SliceStable(authors, func(i, j int) bool {
		ki, kj := keys(authors[i]), keys(authors[j])
		for n := range ki {
			if ki[n] != kj[n] {
				return ki[n] > kj[n]
			}
		}
		return authors[i].Name < authors[j].Name
	})
}

// activity overlays every repo on the same line charts, one color each.
func (m Model) activity(w int) string {
	var lines, commits, prSizes, tests []series
	var pending, prSummaries, testSummaries []string
	for i := range m.repos {
		r := &m.repos[i]
		if s := m.status(r, w); s != "" {
			pending = append(pending, s)
			continue
		}
		h := r.hist
		ls := series{name: r.name, color: r.color}
		for _, p := range h.LinesOverTime {
			ls.times = append(ls.times, p.Start)
			ls.vals = append(ls.vals, float64(p.Lines))
		}
		cs := series{name: r.name, color: r.color}
		for _, b := range h.Weekly {
			cs.times = append(cs.times, b.Start)
			cs.vals = append(cs.vals, float64(b.Commits))
		}
		lines, commits = append(lines, ls), append(commits, cs)

		ps := series{name: r.name, color: r.color}
		for _, mo := range h.PRSizes.Monthly {
			ps.times = append(ps.times, mo.Start)
			ps.vals = append(ps.vals, mo.AvgLines)
		}
		prSizes = append(prSizes, ps)
		prSummaries = append(prSummaries, m.prSizeSummary(r))

		ts := series{name: r.name, color: r.color}
		for _, mo := range h.TestsMonthly {
			ts.times = append(ts.times, mo.Start)
			ts.vals = append(ts.vals, 100*float64(mo.TestCommits)/float64(mo.Commits))
		}
		tests = append(tests, ts)
		testSummaries = append(testSummaries, m.testSummary(r, w))
	}

	var out []string
	if len(pending) > 0 {
		out = append(out, strings.Join(pending, "\n"))
	}
	if len(lines) == 0 {
		return strings.Join(out, "\n")
	}
	if m.comparing() {
		out = append(out, legend(lines))
	}
	from, to, _, _ := m.span() // zero unless windowed, letting the charts fit the data
	out = append(out,
		section(activityCharts[0])+"\n"+timeChart(lines, w, m.chartH(), shortNum, from, to),
		section(activityCharts[1])+"\n"+timeChart(commits, w, m.chartH(), nil, from, to),
		section(activityCharts[2]+" (lines changed)")+"\n"+prSizeChart(prSizes, prSummaries, w, m.chartH(), from, to),
		section(activityCharts[3]+" (% of commits)")+"\n"+testChart(tests, testSummaries, w, m.chartH(), from, to),
		section("When commits happen (local time)")+"\n"+m.punchcards(w),
	)
	return strings.Join(out, "\n")
}

func prSizeChart(ss []series, summaries []string, w, h int, from, to time.Time) string {
	found := false
	for _, s := range ss {
		found = found || len(s.times) > 0
	}
	if !found {
		return dimStyle.Render("No merged PRs found in this branch's history.")
	}
	return timeChart(ss, w, h, shortNum, from, to) + "\n" + strings.Join(summaries, "\n")
}

// prSizeSummary is a one-line overall PR size summary for a repo.
func (m Model) prSizeSummary(r *repoState) string {
	ps := r.hist.PRSizes
	text := "no PRs detected"
	if ps.PRs > 0 {
		text = fmt.Sprintf("%s PRs · mean %s · median %s lines",
			num(ps.PRs), num(int(math.Round(ps.MeanLines))), num(ps.MedianLines))
	}
	if !m.comparing() {
		return text
	}
	return lipgloss.NewStyle().Foreground(r.color).Render("● "+r.name) + "  " + text
}

// punchcards lays repos' heatmaps side by side, narrowing cells if that helps them fit.
func (m Model) punchcards(w int) string {
	cellW := 2
	if m.comparing() && len(m.repos)*(5+24*2+4) > w {
		cellW = 1
	}
	var cards []string
	for i := range m.repos {
		r := &m.repos[i]
		if r.local == nil {
			continue
		}
		card := punchcard(r.hist.Punchcard, cellW)
		if m.comparing() {
			name := lipgloss.NewStyle().Bold(true).Foreground(r.color).Render("● " + r.name)
			card = lipgloss.NewStyle().MarginRight(4).MarginBottom(1).Render(name + "\n" + card)
		}
		cards = append(cards, card)
	}
	return flow(cards, w)
}

type series struct {
	name  string
	color lipgloss.TerminalColor
	times []time.Time
	vals  []float64
}

func legend(ss []series) string {
	var parts []string
	for _, s := range ss {
		parts = append(parts, lipgloss.NewStyle().Foreground(s.color).Render("━━ "+s.name))
	}
	return strings.Join(parts, "   ")
}

// timeChart draws series as braille lines on shared axes. yFmt may be nil for plain integers.
// The x axis covers from..to (when set) and any data outside it, so every repo shares one span.
func timeChart(ss []series, w, h int, yFmt func(float64) string, from, to time.Time) string {
	var drawable []series
	for _, s := range ss {
		if len(s.times) > 1 {
			drawable = append(drawable, s)
		}
	}
	if len(drawable) == 0 {
		return dimStyle.Render("Not enough history for a chart yet.")
	}
	minT, maxT, peak := drawable[0].times[0], drawable[0].times[0], 1.0
	if !from.IsZero() && from.Before(minT) {
		minT = from
	}
	if !to.IsZero() && to.After(maxT) {
		maxT = to
	}
	for _, s := range drawable {
		for i, t := range s.times {
			if t.Before(minT) {
				minT = t
			}
			if t.After(maxT) {
				maxT = t
			}
			peak = math.Max(peak, s.vals[i])
		}
	}
	opts := []timeserieslinechart.Option{
		timeserieslinechart.WithTimeRange(minT, maxT),
		timeserieslinechart.WithYRange(0, peak),
		timeserieslinechart.WithAxesStyles(dimStyle, dimStyle),
	}
	if yFmt != nil {
		opts = append(opts, timeserieslinechart.WithYLabelFormatter(func(_ int, v float64) string { return yFmt(v) }))
	}
	var names []string
	for _, s := range drawable {
		opts = append(opts, timeserieslinechart.WithDataSetStyle(s.name, lipgloss.NewStyle().Foreground(s.color)))
		names = append(names, s.name)
	}
	c := timeserieslinechart.New(w, h, opts...)
	for _, s := range drawable {
		for i, t := range s.times {
			c.PushDataSet(s.name, timeserieslinechart.TimePoint{Time: t, Value: s.vals[i]})
		}
	}
	c.DrawBrailleDataSets(names)
	return c.View()
}

// punchcard draws a weekday × hour heatmap with each hour cellW columns wide.
func punchcard(grid [7][24]int, cellW int) string {
	peak := 0
	for _, row := range grid {
		for _, v := range row {
			peak = max(peak, v)
		}
	}
	glyph := func(level int) string {
		return lipgloss.NewStyle().Foreground(heatRamp[level]).Render(strings.Repeat(heatGlyphs[level], cellW))
	}
	days := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	var b strings.Builder
	b.WriteString("     ")
	for hr := 0; hr < 24; hr += 3 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("%-*d", 3*cellW, hr)))
	}
	b.WriteString("\n")
	for d, row := range grid {
		b.WriteString(dimStyle.Render(days[d]) + "  ")
		for _, v := range row {
			level := 0
			if v > 0 && peak > 0 {
				level = 1 + v*(len(heatRamp)-2)/peak
			}
			b.WriteString(glyph(level))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + dimStyle.Render("     less "))
	for i := range heatRamp {
		b.WriteString(glyph(i))
	}
	b.WriteString(dimStyle.Render(" more"))
	return b.String()
}

// fileListLimit caps the Files tab's lists. History and the line count keep the
// top 50 test and top 50 other files, which covers the top 50 of any filter.
const fileListLimit = 50

func (m Model) filesRepo(r *repoState, w int) []string {
	h, l := r.hist, r.local.LOC
	all, filter := m.showAll(), m.fileFilter
	tag := func(test bool) string {
		if filter != allFiles {
			return "" // every row would say the same
		}
		return testTag(test)
	}

	var hotFiles []stats.File
	for _, f := range h.HotFiles {
		if filter.keep(f.Test) && len(hotFiles) < fileListLimit {
			hotFiles = append(hotFiles, f)
		}
	}
	nHot, hotMore := listCut(len(hotFiles), 15, all)
	var hot []barRow
	for _, f := range hotFiles[:nHot] {
		hot = append(hot, barRow{
			label: f.Path,
			value: float64(f.Commits),
			text: fmt.Sprintf("%5s commits  %s %s", num(f.Commits), addStyle.Render("+"+num(f.Added)), delStyle.Render("-"+num(f.Deleted))) +
				tag(f.Test),
		})
	}
	var largest []loc.File
	for _, f := range l.Largest {
		if filter.keep(f.Test) && len(largest) < fileListLimit {
			largest = append(largest, f)
		}
	}
	nBig, bigMore := listCut(len(largest), 15, all)
	var big []barRow
	for _, f := range largest[:nBig] {
		big = append(big, barRow{
			label: f.Path,
			value: float64(f.Lines),
			text:  fmt.Sprintf("%7s lines", num(f.Lines)) + tag(f.Test),
		})
	}
	title := ""
	if filter != allFiles {
		title = " · " + filter.label()
	}
	sections := []string{
		section("Most frequently changed (still in tree)"+title) + "\n" + withNote(fileChart(hot, w, filter), hotMore),
		section("Largest files"+title+m.treeNote()) + "\n" + withNote(fileChart(big, w, filter), bigMore),
	}
	var left []string
	if r.local.NonCode > 0 {
		left = append(left, fmt.Sprintf("%s data, config and doc files left out (--all-files keeps them).", num(r.local.NonCode)))
	}
	if l.Skipped > 0 {
		left = append(left, fmt.Sprintf("%s tracked files skipped (binary, lockfiles, >4MB, or missing).", num(l.Skipped)))
	}
	if len(left) > 0 {
		sections = append(sections, "\n"+dimStyle.Width(w).Render(strings.Join(left, "\n")))
	}
	return sections
}

// fileChart is a pathBarChart that says when a filter has emptied it.
func fileChart(rows []barRow, w int, filter fileFilter) string {
	if len(rows) == 0 && filter != allFiles {
		return dimStyle.Render("No " + strings.TrimSuffix(filter.label(), " only") + ".")
	}
	return pathBarChart(rows, w)
}

func (m Model) pullRequestsRepo(r *repoState, w int) []string {
	fallback := dimStyle.Width(w).Render(fmt.Sprintf(
		"From commit messages alone, history references ~%s PRs (merge commits and squash-merge titles like \"(#123)\").",
		num(r.hist.PRsInHistory)))
	switch {
	case m.opts.NoRemote:
		return []string{dimStyle.Render("Remote lookups disabled (--no-remote).") + "\n\n" + fallback}
	case !r.prDone:
		return []string{m.spin.View() + " Asking GitHub…\n\n" + fallback}
	case r.prErr != nil:
		msg := lipgloss.NewStyle().Width(w).Render(errStyle.Render("Couldn't load PRs from GitHub: ") + r.prErr.Error())
		return []string{msg + "\n\n" + fallback}
	}

	p := r.prs
	median := "—"
	if p.Sample > 0 {
		median = humanSpan(time.Duration(p.MedianHoursToMerge * float64(time.Hour)))
	}
	tiles := flow([]string{
		tile("Open", num(p.Open)),
		tile("Merged", num(p.Merged)),
		tile("Closed unmerged", num(p.Closed)),
		tile("Median time to merge", median),
	}, w)

	var rows []barRow
	for _, a := range p.TopAuthors[:min(len(p.TopAuthors), 10)] {
		rows = append(rows, barRow{label: a.Login, value: float64(a.Merged), text: num(a.Merged)})
	}
	out := dimStyle.Render(p.Repo) + "\n" + tiles
	if m.windowed() {
		out += "\n" + dimStyle.Width(w).Render("These come from GitHub and cover all time; the time window doesn't apply to them.")
	}
	sections := []string{out}
	if len(rows) > 0 {
		sections = append(sections, section(fmt.Sprintf("Top PR authors (last %d merged)", p.Sample))+"\n"+barChart(rows, w))
	}
	return sections
}
