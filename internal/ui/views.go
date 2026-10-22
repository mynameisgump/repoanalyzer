package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/NimbleMarkets/ntcharts/linechart/timeserieslinechart"
	"github.com/NimbleMarkets/ntcharts/sparkline"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"

	"github.com/ethancrann/repoanalyzer/internal/loc"
	"github.com/ethancrann/repoanalyzer/internal/stats"
)

func section(title string) string {
	return sectionStyle.Render(title)
}

func (m Model) overview(w int) string {
	if !m.comparing() {
		return m.each(w, m.overviewRepo)
	}
	return m.comparison(w) + "\n" + section("Languages (code lines, excl. blank)"+m.treeNote()) + "\n" + m.each(w, languages)
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

	out := tiles + "\n" + section("Languages (code lines, excl. blank)"+m.treeNote()) + "\n" + languages(r, w)
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

func languages(r *repoState, w int) string {
	l := r.local.LOC
	var rows []barRow
	for i, lang := range l.Languages {
		if i == 10 {
			break
		}
		rows = append(rows, barRow{
			label: lang.Name,
			value: float64(lang.Code),
			text:  fmt.Sprintf("%9s  %5s", num(lang.Code), pct(lang.Code, l.Code)),
		})
	}
	out := barChart(rows, w)
	if extra := len(l.Languages) - len(rows); extra > 0 {
		out += "\n" + dimStyle.Render(fmt.Sprintf("+ %d more", extra))
	}
	return out
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
	limit := 50
	if m.comparing() {
		limit = 15 // keep both repos within a reasonable scroll
	}
	windowed := m.windowed()
	return m.each(w, func(r *repoState, w int) string { return authorsRepo(r, w, limit, windowed) })
}

func authorsRepo(r *repoState, w, limit int, windowed bool) string {
	h := r.hist
	if len(h.Authors) == 0 {
		if windowed {
			return dimStyle.Render("No commits in this time window.")
		}
		return dimStyle.Render("No commits yet.")
	}
	recent := "the last 90 days"
	if windowed {
		recent = "the window's last 90 days"
	}
	summary := fmt.Sprintf("%s contributors · %s active in %s",
		boldStyle.Render(num(len(h.Authors))), boldStyle.Render(num(h.ActiveAuthors90d)), recent)

	authors := h.Authors[:min(len(h.Authors), limit)]
	// On narrow terminals drop the share bar and first-commit date so the table fits.
	wide := w >= 110
	nameW := min(28, max(w/4, 12))
	headers := []string{"#", "Author", "Commits", "Share", "Added", "Deleted", "Last"}
	if wide {
		headers = []string{"#", "Author", "Commits", "Share", "Added", "Deleted", "First", "Last"}
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
	topCommits := float64(authors[0].Commits)
	for i, a := range authors {
		share := pct(a.Commits, h.Commits)
		row := []string{fmt.Sprint(i + 1), truncate(a.Name, nameW), num(a.Commits), share,
			"+" + num(a.Added), "-" + num(a.Deleted), ago(a.Last)}
		if wide {
			row[3] = hbar(float64(a.Commits), topCommits, 10) + " " + share
			row = append(row[:6], a.First.Format("2006-01-02"), ago(a.Last))
		}
		t.Row(row...)
	}
	rendered := t.Render()
	if lipgloss.Width(rendered) > w {
		rendered = t.Width(w).Render() // let lipgloss squeeze the columns
	}
	out := summary + "\n" + rendered
	if len(h.Authors) > limit {
		out += "\n" + dimStyle.Render(fmt.Sprintf("+ %d more (use --json for the full list)", len(h.Authors)-limit))
	}
	if len(h.PRSizes.ByAuthor) > 0 {
		out += "\n" + section("PR size by author (lines changed per PR)") + "\n" + prSizesByAuthor(h.PRSizes, w, min(limit, 10))
	}
	return out
}

// prSizesByAuthor charts each author's average PR size, most prolific first.
func prSizesByAuthor(ps stats.PRSizeStats, w, limit int) string {
	authors := ps.ByAuthor[:min(len(ps.ByAuthor), limit)]
	labelW := 0
	for _, a := range authors {
		labelW = max(labelW, min(lipgloss.Width(a.Name), 24))
	}
	// Pad each figure to its column so rows line up.
	var avgW, medW, prsW int
	for _, a := range authors {
		avgW = max(avgW, len(num(int(math.Round(a.MeanLines)))))
		medW = max(medW, len(num(a.MedianLines)))
		prsW = max(prsW, len(num(a.PRs)))
	}
	texts := func(withMedian bool) ([]string, int) {
		var out []string
		textW := 0
		for _, a := range authors {
			avg := int(math.Round(a.MeanLines))
			// "line " keeps the singular as wide as "lines" so the columns after it stay aligned.
			text := fmt.Sprintf("avg %*s line%s", avgW, num(avg), plural(avg, "s", " "))
			if withMedian {
				text += fmt.Sprintf(" · median %*s", medW, num(a.MedianLines))
			}
			text += fmt.Sprintf(" · %*s PR%s", prsW, num(a.PRs), plural(a.PRs, "s", ""))
			out = append(out, text)
			textW = max(textW, len(text))
		}
		return out, textW
	}
	// Drop the median if keeping it would squeeze the bars too much.
	text, textW := texts(true)
	if w-labelW-textW-4 < 10 {
		text, _ = texts(false)
	}
	var rows []barRow
	for i, a := range authors {
		rows = append(rows, barRow{label: a.Name, value: a.MeanLines, text: text[i]})
	}
	out := barChart(rows, w)
	if more := len(ps.ByAuthor) - len(authors); more > 0 {
		out += "\n" + dimStyle.Render(fmt.Sprintf("+ %d more (use --json for the full list)", more))
	}
	return out + "\n" + dimStyle.Width(w).Render("Lines added + deleted per PR merged into this branch (excl. lockfiles). "+
		"Merge commits count for the PR branch's author, not whoever merged. A few huge PRs pull the average up; the median is the typical PR.")
}

// plural returns many unless n is 1, in which case it returns one.
func plural(n int, many, one string) string {
	if n == 1 {
		return one
	}
	return many
}

// activity overlays every repo on the same line charts, one color each.
func (m Model) activity(w int) string {
	var lines, commits, prSizes []series
	var pending, prSummaries []string
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
		section(activityCharts[0])+"\n"+timeChart(lines, w, m.chartH(), shortNum, from, to)+"\n"+
			dimStyle.Width(w).Render("All lines in tracked text files (incl. blank), rebuilt from per-commit diffs."),
		section(activityCharts[1])+"\n"+timeChart(commits, w, m.chartH(), nil, from, to),
		section(activityCharts[2]+" (lines changed)")+"\n"+prSizeChart(prSizes, prSummaries, w, m.chartH(), from, to),
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
		return dimStyle.Width(w).Render("No merged PRs found in this branch's history. PRs are detected from " +
			"\"Merge pull request #N\" commits and squash-merge titles ending in \"(#N)\"; rebase merges can't be detected.")
	}
	return timeChart(ss, w, h, shortNum, from, to) + "\n" + strings.Join(summaries, "\n") + "\n" +
		dimStyle.Width(w).Render("Lines added + deleted per PR as it landed on this branch (excl. lockfiles), "+
			"averaged per month; months without PRs are skipped. Large PRs pull the average up, so compare with the median.")
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

func (m Model) filesRepo(r *repoState, w int) string {
	h, l := r.hist, r.local.LOC

	var hot []barRow
	for _, f := range h.HotFiles[:min(len(h.HotFiles), 15)] {
		hot = append(hot, barRow{
			label: f.Path,
			value: float64(f.Commits),
			text:  fmt.Sprintf("%5s commits  %s %s", num(f.Commits), addStyle.Render("+"+num(f.Added)), delStyle.Render("-"+num(f.Deleted))),
		})
	}
	var big []barRow
	for _, f := range l.Largest {
		big = append(big, barRow{
			label: f.Path,
			value: float64(f.Lines),
			text:  fmt.Sprintf("%7s lines", num(f.Lines)),
		})
	}
	out := section("Most frequently changed (still in tree)") + "\n" + pathBarChart(hot, w) + "\n" +
		section("Largest files"+m.treeNote()) + "\n" + pathBarChart(big, w) + "\n" +
		section("Tests vs non-test code"+m.treeNote()) + "\n" + testSplit(l, w)
	if l.Skipped > 0 {
		out += "\n\n" + dimStyle.Render(fmt.Sprintf("%s tracked files skipped (binary, lockfiles, >4MB, or missing).", num(l.Skipped)))
	}
	return out
}

// testSplit shows how much of the tree is tests, overall and per language.
// Line counts only cover programming languages: a JSON fixture or a README in
// a test folder says little about how well the code is tested.
func testSplit(l loc.Result, w int) string {
	if l.TestFiles == 0 {
		return dimStyle.Width(w).Render("No test files found. Looked for test directories (tests/, __tests__/, spec/, …) " +
			"and names like foo_test.go, foo.test.ts, test_foo.py, FooTest.java.")
	}
	var code, testCode int
	var langs []loc.Language
	for _, lang := range l.Languages {
		if !loc.IsCode(lang.Name) {
			continue
		}
		code += lang.Code
		testCode += lang.TestCode
		if lang.TestFiles > 0 && lang.Code > 0 && len(langs) < 8 {
			langs = append(langs, lang)
		}
	}
	tiles := flow([]string{
		tile("Test files", fmt.Sprintf("%s of %s · %s", num(l.TestFiles), num(l.Files), pct(l.TestFiles, l.Files))),
		tile("Test code lines", fmt.Sprintf("%s of %s · %s", num(testCode), num(code), pct(testCode, code))),
	}, w)
	note := dimStyle.Width(w).Render("Test files include fixtures and other files under test directories. Lines count " +
		"only programming languages (excl. blank), not JSON, YAML, Markdown and the like. Tests written inline in source " +
		"files (e.g. Rust's #[cfg(test)]) count as non-test.")
	if len(langs) == 0 {
		return tiles + "\n" + note
	}

	type row struct {
		label, text, files string
		share              float64
	}
	var rows []row
	labelW, textW, filesW := 0, 0, 0
	for _, lang := range langs {
		r := row{
			label: truncate(lang.Name, 16),
			share: float64(lang.TestCode) / float64(lang.Code),
			text:  fmt.Sprintf("%5s  %s of %s lines", pct(lang.TestCode, lang.Code), num(lang.TestCode), num(lang.Code)),
			files: fmt.Sprintf("  · %s of %s files", num(lang.TestFiles), num(lang.Files)),
		}
		rows = append(rows, r)
		labelW, textW, filesW = max(labelW, lipgloss.Width(r.label)), max(textW, lipgloss.Width(r.text)), max(filesW, lipgloss.Width(r.files))
	}
	// Drop the file counts when the bar would get too short to read.
	withFiles := w-labelW-textW-filesW-4 >= 16
	barW := w - labelW - textW - 4
	if withFiles {
		barW -= filesW
	}
	barW = max(barW, 4)

	var b strings.Builder
	for _, r := range rows {
		filled := int(math.Round(r.share * float64(barW)))
		if filled == 0 && r.share > 0 {
			filled = 1
		}
		text := lipgloss.NewStyle().Width(textW).Render(r.text)
		if withFiles {
			text += r.files
		}
		fmt.Fprintf(&b, "%s  %s%s %s\n",
			lipgloss.NewStyle().Width(labelW).Render(r.label),
			addStyle.Render(strings.Repeat("█", filled)),
			dimStyle.Render(strings.Repeat("░", barW-filled)),
			dimStyle.Render(text))
	}
	return tiles + "\n" + section("Share of each language's code that is tests") + "\n" +
		strings.TrimRight(b.String(), "\n") + "\n" + note
}

func (m Model) pullRequestsRepo(r *repoState, w int) string {
	fallback := dimStyle.Width(w).Render(fmt.Sprintf(
		"From commit messages alone, history references ~%s PRs (merge commits and squash-merge titles like \"(#123)\").",
		num(r.hist.PRsInHistory)))
	switch {
	case m.opts.NoRemote:
		return dimStyle.Render("Remote lookups disabled (--no-remote).") + "\n\n" + fallback
	case !r.prDone:
		return m.spin.View() + " Asking GitHub…\n\n" + fallback
	case r.prErr != nil:
		msg := lipgloss.NewStyle().Width(w).Render(errStyle.Render("Couldn't load PRs from GitHub: ") + r.prErr.Error())
		return msg + "\n\n" + fallback
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
	if len(rows) > 0 {
		out += "\n" + section(fmt.Sprintf("Top PR authors (last %d merged)", p.Sample)) + "\n" + barChart(rows, w)
	}
	return out
}
