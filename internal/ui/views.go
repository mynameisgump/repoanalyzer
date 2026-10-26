package ui

import (
	"fmt"
	"math"
	"math/bits"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/NimbleMarkets/ntcharts/canvas"
	"github.com/NimbleMarkets/ntcharts/canvas/graph"
	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	"github.com/NimbleMarkets/ntcharts/linechart/timeserieslinechart"
	"github.com/NimbleMarkets/ntcharts/sparkline"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"

	"github.com/mynameisgump/repoanalyzer/internal/loc"
	"github.com/mynameisgump/repoanalyzer/internal/remote"
	"github.com/mynameisgump/repoanalyzer/internal/stats"
)

func section(title string) string {
	return sectionStyle.Render(title)
}

// overview is each repo's tiles, languages and weekly commits: side by side in
// columns when comparing with c on, else stacked.
func (m Model) overview(w int) string {
	widths := m.tileWidths(m.overviewFigures)
	return m.each(w, func(r *repoState, w int) []string { return m.overviewRepo(r, w, widths) })
}

func (m Model) overviewRepo(r *repoState, w int, widths []int) []string {
	sections := []string{
		flow(figureTiles(m.overviewFigures(r), widths), w),
		section("Languages (code lines, excl. blank)"+m.treeNote()) + "\n" + languages(r, w, m.showAll()),
	}
	if h := r.hist; len(h.Weekly) > 1 {
		sections = append(sections, section("Commits per week")+"\n"+weeklySparkline(h.Weekly, w, r.color))
	}
	return sections
}

// figure is one tile's label and value.
type figure struct{ label, value string }

// tileWidths sizes each of figures' tiles to its widest version in any loaded
// repo, so side by side the repos' grids wrap the same way and every tile
// lines up with its counterpart.
func (m Model) tileWidths(figures func(*repoState) []figure) []int {
	var widths []int
	for i := range m.repos {
		r := &m.repos[i]
		if r.local == nil || r.localErr != nil {
			continue
		}
		for k, f := range figures(r) {
			if k == len(widths) {
				widths = append(widths, 0)
			}
			widths[k] = max(widths[k], lipgloss.Width(f.label), lipgloss.Width(f.value))
		}
	}
	return widths
}

// figureTiles draws figures as tiles with the widths from tileWidths.
func figureTiles(fs []figure, widths []int) []string {
	var tiles []string
	for k, f := range fs {
		tiles = append(tiles, tileStyle.Width(widths[k]+2).Render(dimStyle.Render(f.label)+"\n"+tileValueStyle.Render(f.value)))
	}
	return tiles
}

// overviewFigures are a repo's tiles on the overview. Every repo gets the same
// ones in the same order, so side by side they line up.
func (m Model) overviewFigures(r *repoState) []figure {
	h := r.hist
	// Age is the repo's, not the time window's.
	age := "—"
	if r.local.History.Commits > 0 {
		age = humanSpan(time.Since(r.local.History.First))
	}
	prLabel := "Merged PRs"
	if r.prs == nil && r.prDone || m.opts.NoRemote {
		prLabel = "PRs (from history)"
	}
	prSize := "—"
	if h.PRSizes.PRs > 0 {
		prSize = num(h.PRSizes.MedianLines) + " lines"
	}
	testShare := "—"
	if h.Tests.Added > 0 {
		testShare = pct(h.Tests.TestAdded, h.Tests.Added)
	}
	figures := []figure{
		{"Lines of code", num(r.local.LOC.Code)},
		{"Files", num(r.local.LOC.Files)},
		{"Commits", num(h.Commits)},
		{"Last commit", ago(h.Last)},
		{"Lines added", "+" + num(h.Added)},
		{"Lines deleted", "-" + num(h.Deleted)},
		{"Contributors", num(len(h.Authors))},
		{"Active (90d)", num(h.ActiveAuthors90d)},
		{"Repo age", age},
		{prLabel, m.prValue(r)},
	}
	if m.anyGitHub() {
		figures = append(figures, figure{"Open PRs", m.gitHubValue(r, func(p *remote.PRStats) string { return num(p.Open) })})
	}
	figures = append(figures, figure{"Median PR size", prSize})
	if m.anyGitHub() {
		figures = append(figures, figure{"Median time to merge", m.gitHubValue(r, func(p *remote.PRStats) string {
			if p.Sample == 0 {
				return "—"
			}
			return humanSpan(time.Duration(p.MedianHoursToMerge * float64(time.Hour)))
		})})
	}
	return append(figures, figure{"Lines added to tests", testShare})
}

// anyGitHub reports whether any repo has, or may yet get, PR stats from GitHub;
// the figures only GitHub knows are left off the overview otherwise.
func (m Model) anyGitHub() bool {
	if m.opts.NoRemote {
		return false
	}
	for _, r := range m.repos {
		if r.prs != nil || !r.prDone {
			return true
		}
	}
	return false
}

// gitHubValue is a figure from a repo's GitHub PR stats, "…" while they load
// and "—" if they couldn't be.
func (m Model) gitHubValue(r *repoState, value func(*remote.PRStats) string) string {
	switch {
	case r.prs != nil:
		return value(r.prs)
	case !r.prDone:
		return "…"
	}
	return "—"
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

// authors is each repo's contributor tiles, commits per week by its top
// authors, active contributors per week, and the contributors table.
func (m Model) authors(w int) string {
	limit, windowed, all := m.authorsLimit(), m.windowed(), m.showAll()
	colW := w
	if m.columns() {
		colW = m.columnWidth()
	}
	headers, _ := m.authorColumns(colW)
	widths := m.tileWidths(m.authorFigures)
	// Every repo's charts share the same axes, so side by side they can be read
	// against each other. Each repo's top authors have their own scale.
	from, to, _, _ := m.span()
	byAuthor := chartBounds{from: from, to: to}
	active := byAuthor
	for i := range m.repos {
		r := &m.repos[i]
		if r.local == nil || r.localErr != nil {
			continue
		}
		for _, wk := range r.hist.AuthorsWeekly {
			active.peak = math.Max(active.peak, float64(wk.Active))
		}
	}
	return m.each(w, func(r *repoState, w int) []string {
		return m.authorsRepo(r, w, limit, windowed, all, headers, widths, byAuthor, active)
	})
}

// topAuthorsCharted is how many authors Commits by author overlays.
const topAuthorsCharted = 5

// authorFigures are a repo's tiles on Authors. Every repo gets the same ones
// in the same order, so side by side they line up.
func (m Model) authorFigures(r *repoState) []figure {
	h := r.hist
	// Walk authors most-commits-first until they cover half the commits.
	single, half, covered := 0, 0, 0
	for _, a := range h.Authors {
		if a.Commits == 1 {
			single++
		}
		if covered*2 < h.Commits {
			half++
			covered += a.Commits
		}
	}
	// New contributors are counted from all-time history, like Active (90d)
	// they describe a fixed recent period rather than the window.
	_, to, ok, _ := m.span()
	if !ok {
		to = time.Now()
	}
	newAuthors := 0
	for _, a := range r.local.History.Authors {
		if a.First.After(to.AddDate(-1, 0, 0)) && !a.First.After(to) {
			newAuthors++
		}
	}
	top, busFactor := "—", "—"
	if len(h.Authors) > 0 {
		top = pct(h.Authors[0].Commits, h.Commits)
		busFactor = num(half)
	}
	return []figure{
		{"Contributors", num(len(h.Authors))},
		{"Active (90d)", num(h.ActiveAuthors90d)},
		{"New (1y)", num(newAuthors)},
		{"Single-commit authors", num(single)},
		{"Authors for 50% of commits", busFactor},
		{"Top author share", top},
	}
}

// activeAuthorsChart plots how many authors committed each week.
func activeAuthorsChart(r *repoState, w, height int, b chartBounds) string {
	s := series{name: r.name, color: r.color}
	for _, wk := range r.hist.AuthorsWeekly {
		s.times = append(s.times, wk.Start)
		s.vals = append(s.vals, float64(wk.Active))
	}
	return timeChart([]series{s}, w, height, nil, b)
}

// authorsLimit is how many authors the table shows before e expands it.
func (m Model) authorsLimit() int {
	if m.comparing() {
		return 15 // keep both repos within a reasonable scroll
	}
	return 50
}

// authorColumns picks the Authors table's columns for tables w wide. Every repo
// gets the same ones: First, then Share, PRs and Last, are dropped until the
// widest repo's table fits. Direct (commits that didn't come through a PR) and
// Email are there only once x asks for more details, and then they're the last
// columns to go. fits is false when even the fewest columns are too wide, and
// the tables will be squeezed, wrapping names and numbers.
func (m Model) authorColumns(w int) (headers []string, fits bool) {
	headers = []string{"#", "Author", "Email", "Commits", "PRs", "Direct", "Share", "Added", "Deleted", "Last"}
	if w >= 110 { // room for the share bar and the first commit's date
		headers = []string{"#", "Author", "Email", "Commits", "PRs", "Direct", "Share", "Added", "Deleted", "First", "Last"}
	}
	drops := []string{"", "First", "Share", "PRs", "Last", "Direct", "Email"}
	if !m.details {
		headers = slices.DeleteFunc(headers, func(h string) bool { return h == "Email" || h == "Direct" })
		drops = drops[:len(drops)-2]
	}
	if !m.anyPRsInHistory() {
		headers = slices.DeleteFunc(headers, func(h string) bool { return h == "PRs" || h == "Direct" })
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
	for _, drop := range drops {
		if drop != "" {
			headers = slices.DeleteFunc(slices.Clone(headers), func(h string) bool { return h == drop })
		}
		if fit(headers) {
			return headers, true
		}
	}
	return headers, false
}

// anyPRsInHistory reports whether any loaded repo's history has merged PRs,
// which is when the Authors table gets a PRs column.
func (m Model) anyPRsInHistory() bool {
	for _, r := range m.repos {
		if r.local != nil && r.hist.PRSizes.PRs > 0 {
			return true
		}
	}
	return false
}

func (m Model) authorsRepo(r *repoState, w, limit int, windowed, all bool, headers []string, widths []int, byAuthor, active chartBounds) []string {
	h := r.hist
	if len(h.Authors) == 0 {
		if windowed {
			return []string{dimStyle.Render("No commits in this time window.")}
		}
		return []string{dimStyle.Render("No commits yet.")}
	}
	_, more := listCut(len(h.Authors), limit, all)
	t := authorsTable(r, w, limit, all, headers)
	rendered := t.Render()
	if lipgloss.Width(rendered) > w {
		rendered = t.Width(w).Render() // let lipgloss squeeze the columns
	}
	return []string{
		flow(figureTiles(m.authorFigures(r), widths), w),
		m.byAuthorTitle() + "\n" +
			m.commitsByAuthorChart(r, w, byAuthor),
		section(authorCharts[1]) + "\n" + activeAuthorsChart(r, w, m.chartH(), active),
		section("Contributors by commits") + "\n" + withNote(rendered, more),
	}
}

// authorsTable is a repo's contributors table with the given columns (see authorColumns).
func authorsTable(r *repoState, w, limit int, all bool, headers []string) *table.Table {
	h := r.hist
	n, _ := listCut(len(h.Authors), limit, all)
	authors := h.Authors[:n]
	wide := slices.Contains(headers, "First")
	nameW := min(28, max(w/4, 12))
	emailW := min(32, max(w/5, 12))
	topCommits := float64(authors[0].Commits)
	prs, direct := map[string]int{}, map[string]int{}
	for _, p := range h.PRSizes.ByAuthor {
		prs[strings.ToLower(p.Name)] = p.PRs
	}
	for _, l := range h.Landings.ByAuthor {
		direct[strings.ToLower(l.Name)] = l.Direct()
	}
	cell := func(i int, a stats.Author, col string) string {
		switch col {
		case "#":
			return fmt.Sprint(i + 1)
		case "Author":
			return truncate(a.Name, nameW)
		case "Email":
			return truncate(a.Email, emailW)
		case "Commits":
			return num(a.Commits)
		case "PRs":
			return num(prs[strings.ToLower(a.Name)])
		case "Direct":
			return num(direct[strings.ToLower(a.Name)])
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
			case "Commits", "PRs", "Direct":
				s = s.Align(lipgloss.Right)
			case "Added":
				s = s.Align(lipgloss.Right).Foreground(green)
			case "Deleted":
				s = s.Align(lipgloss.Right).Foreground(red)
			case "Email", "First", "Last":
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
	var lines, commits, tests []series
	var pending, testSummaries []string
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
		section(activityCharts[0])+"\n"+timeChart(lines, w, m.chartH(), shortNum, chartBounds{from: from, to: to}),
		section(activityCharts[1])+"\n"+timeChart(commits, w, m.chartH(), nil, chartBounds{from: from, to: to}),
		section(activityCharts[2]+" (% of commits)")+"\n"+testChart(tests, testSummaries, w, m.chartH(), from, to),
		section("When commits happen (local time)")+"\n"+m.punchcards(w),
	)
	return strings.Join(out, "\n")
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
	// over gives the line every cell it shares with others; see drawLines.
	over bool
}

func legend(ss []series) string {
	var parts []string
	for _, s := range ss {
		parts = append(parts, lipgloss.NewStyle().Foreground(s.color).Render("━━ "+s.name))
	}
	return strings.Join(parts, "   ")
}

// chartBounds widens a chart's axes past its own data: the x axis to cover
// from..to and the y axis to reach peak, where set. Separate charts given the
// same bounds can be read against each other.
type chartBounds struct {
	from, to time.Time
	peak     float64
}

// timeChart draws series as braille lines on shared axes. yFmt may be nil for plain integers.
// The axes cover b and all the data, so every repo shares one span.
func timeChart(ss []series, w, h int, yFmt func(float64) string, b chartBounds) string {
	return lineChart(ss, w, h, yFmt, b, drawLines)
}

// blockChart is timeChart drawn in half blocks; see blockLines.
func blockChart(ss []series, w, h int, yFmt func(float64) string, b chartBounds) string {
	return lineChart(ss, w, h, yFmt, b, blockLines)
}

// lineChart sets up a timeChart's axes and has draw draw the lines on them.
func lineChart(ss []series, w, h int, yFmt func(float64) string, b chartBounds, draw func(*timeserieslinechart.Model, []series)) string {
	var drawable []series
	for _, s := range ss {
		if len(s.times) > 1 {
			drawable = append(drawable, s)
		}
	}
	if len(drawable) == 0 {
		return dimStyle.Render("Not enough history for a chart yet.")
	}
	minT, maxT, peak := drawable[0].times[0], drawable[0].times[0], b.peak
	if !b.from.IsZero() && b.from.Before(minT) {
		minT = b.from
	}
	if !b.to.IsZero() && b.to.After(maxT) {
		maxT = b.to
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
	if peak <= 0 {
		peak = 1 // all zeros: the axis still needs a height
	}
	opts := []timeserieslinechart.Option{
		timeserieslinechart.WithTimeRange(minT, maxT),
		timeserieslinechart.WithYRange(0, peak),
		timeserieslinechart.WithAxesStyles(dimStyle, dimStyle),
	}
	if yFmt != nil {
		opts = append(opts, timeserieslinechart.WithYLabelFormatter(func(_ int, v float64) string { return yFmt(v) }))
	}
	c := timeserieslinechart.New(w, h, opts...)
	draw(&c, drawable)
	return c.View()
}

// blockLines draws each series on c's axes as a line of half blocks. Every
// cell is two pixels, top and bottom, each with its own color, so lines that
// pass through the same cell both show. Where two need the same pixel, a line
// marked over gets it, else the earlier series.
func blockLines(c *timeserieslinechart.Model, ss []series) {
	c.Clear()
	c.DrawXYAxisAndLabel()
	gw, gh := c.GraphWidth(), c.GraphHeight()*2
	owner := make([][]int, gh) // the index of the series drawn on each pixel, or -1
	for y := range owner {
		owner[y] = slices.Repeat([]int{-1}, gw)
	}
	pixel := func(t time.Time, v float64) canvas.Point {
		x := (float64(t.Unix()) - c.ViewMinX()) / (c.ViewMaxX() - c.ViewMinX()) * float64(gw-1)
		y := (v - c.ViewMinY()) / (c.ViewMaxY() - c.ViewMinY()) * float64(gh-1)
		return canvas.Point{X: int(math.Round(x)), Y: gh - 1 - int(math.Round(y))}
	}
	for i, s := range ss {
		var prev canvas.Point
		for j, t := range s.times {
			p := pixel(t, s.vals[j])
			if j == 0 {
				prev = p
			}
			for _, q := range graph.GetLinePoints(prev, p) {
				if q.X < 0 || q.X >= gw || q.Y < 0 || q.Y >= gh {
					continue
				}
				if o := owner[q.Y][q.X]; o < 0 || s.over && !ss[o].over {
					owner[q.Y][q.X] = i
				}
			}
			prev = p
		}
	}
	left := 0
	if c.YStep() > 0 {
		left = c.Origin().X + 1
	}
	for y := range c.GraphHeight() {
		for x := range gw {
			top, bottom := owner[2*y][x], owner[2*y+1][x]
			var r rune
			style := lipgloss.NewStyle()
			switch {
			case top < 0 && bottom < 0:
				continue
			case top == bottom:
				r, style = '█', style.Foreground(ss[top].color)
			case bottom < 0:
				r, style = '▀', style.Foreground(ss[top].color)
			case top < 0:
				r, style = '▄', style.Foreground(ss[bottom].color)
			default:
				r, style = '▀', style.Foreground(ss[top].color).Background(ss[bottom].color)
			}
			c.Canvas.SetCell(canvas.Point{X: left + x, Y: y}, canvas.NewCellWithStyle(r, style))
		}
	}
}

// drawLines draws each series on c's axes as a braille line. A cell can only
// have one color, so where lines share one, one line gets the cell and only
// its dots are drawn: a line marked over, else the one with more dots there
// (so a spike keeps its foot where it crosses a flat line), else the earlier
// series. Merging the dots, as ntcharts' DrawBrailleDataSets does, would show
// one line's dots in another's color.
func drawLines(c *timeserieslinechart.Model, ss []series) {
	c.Clear()
	c.DrawXYAxisAndLabel()
	gw, gh := c.GraphWidth(), c.GraphHeight()
	xs := float64(gw) / (c.ViewMaxX() - c.ViewMinX())
	ys := float64(gh) / (c.ViewMaxY() - c.ViewMinY())
	left := 0
	if c.YStep() > 0 {
		left = c.Origin().X + 1
	}
	patterns := make([][][]rune, len(ss))
	for i, s := range ss {
		g := graph.NewBrailleGrid(gw, gh, 0, float64(gw), 0, float64(gh))
		var prev canvas.Point
		for j, t := range s.times {
			p := g.GridPoint(canvas.Float64Point{
				X: (float64(t.Unix()) - c.ViewMinX()) * xs,
				Y: (s.vals[j] - c.ViewMinY()) * ys,
			})
			if j == 0 {
				prev = p
			}
			for _, q := range graph.GetLinePoints(prev, p) {
				g.Set(q)
			}
			prev = p
		}
		patterns[i] = g.BraillePatterns()
	}
	dots := func(r rune) int { return bits.OnesCount(uint(r - runes.BrailleBlockOffset)) }
	for y := range gh {
		for x := range gw {
			best := -1
			for i, s := range ss {
				r := patterns[i][y][x]
				if r == runes.BrailleBlockOffset {
					continue
				}
				if best < 0 || s.over && !ss[best].over ||
					s.over == ss[best].over && dots(r) > dots(patterns[best][y][x]) {
					best = i
				}
			}
			if best >= 0 {
				style := lipgloss.NewStyle().Foreground(ss[best].color)
				c.Canvas.SetCell(canvas.Point{X: left + x, Y: y}, canvas.NewCellWithStyle(patterns[best][y][x], style))
			}
		}
	}
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

// pullRequests is each repo's PR figures, PR sizes over time and by author,
// and top PR authors from GitHub.
func (m Model) pullRequests(w int) string {
	widths := m.tileWidths(m.prFigures)
	// Every repo's chart has the same axes, so they can be read side by side.
	from, to, _, _ := m.span()
	b := chartBounds{from: from, to: to}
	for i := range m.repos {
		r := &m.repos[i]
		if r.local == nil || r.localErr != nil {
			continue
		}
		for _, mo := range r.hist.PRSizes.Monthly {
			if b.from.IsZero() || mo.Start.Before(b.from) {
				b.from = mo.Start
			}
			if b.to.IsZero() || mo.Start.After(b.to) {
				b.to = mo.Start
			}
			b.peak = math.Max(b.peak, mo.AvgLines)
		}
	}
	// The share chart runs 0–100% over the window, or over every repo's months.
	share := chartBounds{from: from, to: to, peak: 100}
	for i := range m.repos {
		r := &m.repos[i]
		if r.local == nil || r.localErr != nil {
			continue
		}
		for _, mo := range r.hist.Landings.Monthly {
			if share.from.IsZero() || mo.Start.Before(share.from) {
				share.from = mo.Start
			}
			if share.to.IsZero() || mo.Start.After(share.to) {
				share.to = mo.Start
			}
		}
	}
	return m.each(w, func(r *repoState, w int) []string { return m.pullRequestsRepo(r, w, widths, b, share) })
}

// landingsChart plots the share of each month's commits that came through a PR.
func landingsChart(r *repoState, w, height int, b chartBounds) string {
	s := series{name: r.name, color: r.color}
	for _, mo := range r.hist.Landings.Monthly {
		s.times = append(s.times, mo.Start)
		s.vals = append(s.vals, 100*float64(mo.ViaPR)/float64(mo.Commits))
	}
	return timeChart([]series{s}, w, height, percent, b)
}

func (m Model) pullRequestsRepo(r *repoState, w int, widths []int, b, share chartBounds) []string {
	var status string
	switch {
	case m.opts.NoRemote:
		status = dimStyle.Render("Remote lookups disabled (--no-remote).")
	case !r.prDone:
		status = m.spin.View() + " Asking GitHub…"
	case r.prErr != nil:
		status = lipgloss.NewStyle().Width(w).Render(errStyle.Render("Couldn't load PRs from GitHub: ") + r.prErr.Error())
	default:
		status = dimStyle.Render(r.prs.Repo)
	}
	top := status + "\n" + flow(figureTiles(m.prFigures(r), widths), w)
	if m.windowed() && r.prs != nil {
		top += "\n" + dimStyle.Width(w).Render("Open, merged, closed and time to merge come from GitHub and cover all time; the time window doesn't apply to them.")
	}

	h := r.hist
	chart := dimStyle.Render("No merged PRs found in this branch's history.")
	if len(h.PRSizes.Monthly) > 0 {
		s := series{name: r.name, color: r.color}
		for _, mo := range h.PRSizes.Monthly {
			s.times = append(s.times, mo.Start)
			s.vals = append(s.vals, mo.AvgLines)
		}
		chart = timeChart([]series{s}, w, m.chartH(), shortNum, b)
	}
	byAuthor := dimStyle.Render("No merged PRs found in this branch's history.")
	if len(h.PRSizes.ByAuthor) > 0 {
		byAuthor = prSizesByAuthor(h.PRSizes, w, 10, m.showAll(), m.prSort)
	}
	landings := dimStyle.Render("No merged PRs found in this branch's history.")
	if h.PRSizes.PRs > 0 && len(h.Landings.Monthly) > 0 {
		landings = landingsChart(r, w, m.chartH(), share)
	}
	sections := []string{
		top,
		section(prCharts[0]+" (lines changed)") + "\n" + chart,
		section(prCharts[1]+" (% of commits)") + "\n" + landings,
		section("PR size by author (lines changed per PR)") + "\n" + byAuthor,
	}

	if r.prs != nil && len(r.prs.TopAuthors) > 0 {
		var rows []barRow
		for _, a := range r.prs.TopAuthors[:min(len(r.prs.TopAuthors), 10)] {
			rows = append(rows, barRow{label: a.Login, value: float64(a.Merged), text: num(a.Merged)})
		}
		sections = append(sections, section(fmt.Sprintf("Top PR authors (last %d merged)", r.prs.Sample))+"\n"+barChart(rows, w))
	}
	return sections
}

// prFigures are a repo's tiles on Pull Requests: GitHub's counts when any
// repo can have them, then PR sizes from history.
func (m Model) prFigures(r *repoState) []figure {
	var figures []figure
	if m.anyGitHub() {
		figures = append(figures,
			figure{"Open", m.gitHubValue(r, func(p *remote.PRStats) string { return num(p.Open) })},
			figure{"Merged", m.gitHubValue(r, func(p *remote.PRStats) string { return num(p.Merged) })},
			figure{"Closed unmerged", m.gitHubValue(r, func(p *remote.PRStats) string { return num(p.Closed) })},
			figure{"Median time to merge", m.gitHubValue(r, func(p *remote.PRStats) string {
				if p.Sample == 0 {
					return "—"
				}
				return humanSpan(time.Duration(p.MedianHoursToMerge * float64(time.Hour)))
			})},
		)
	}
	ps, l := r.hist.PRSizes, r.hist.Landings
	mean, median, viaPR, direct := "—", "—", "—", "—"
	if ps.PRs > 0 {
		mean = num(int(math.Round(ps.MeanLines))) + " lines"
		median = num(ps.MedianLines) + " lines"
	}
	// Without a single PR marker every commit looks direct, which says more
	// about how PRs are merged than about pushes; see LandingStats.
	if ps.PRs > 0 && l.Commits > 0 {
		viaPR = pct(l.ViaPR, l.Commits)
		direct = num(l.Direct())
	}
	return append(figures,
		figure{"PRs in history", "~" + num(r.hist.PRsInHistory)},
		figure{"Mean PR size", mean},
		figure{"Median PR size", median},
		figure{"Commits via PR", viaPR},
		figure{"Commits outside PRs", direct},
	)
}
