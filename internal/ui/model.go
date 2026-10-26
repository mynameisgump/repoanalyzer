// Package ui is the Bubble Tea dashboard.
package ui

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/mynameisgump/repoanalyzer/internal/remote"
	"github.com/mynameisgump/repoanalyzer/internal/report"
	"github.com/mynameisgump/repoanalyzer/internal/stats"
)

// MaxRepos is how many repos can be compared at once (one color each).
var MaxRepos = len(repoColors)

type Options struct {
	Roots    []string // repo top-level directories; more than one compares them
	Since    string
	NoRemote bool
	AllFiles bool // keep data, config and docs in the stats
}

var tabNames = []string{"Overview", "Authors", "Activity", "Files", "Pull Requests"}

const (
	authorsTab  = 1
	activityTab = 2
	filesTab    = 3
	prTab       = 4
)

// repoState is one repo's data as it arrives.
type repoState struct {
	name     string
	color    lipgloss.AdaptiveColor
	local    *report.Local
	localErr error
	// hist is local.History limited to the current time window; views read this.
	hist    stats.Summary
	windows map[windowMode]stats.Summary // hist per window, computed on first use
	prs     *remote.PRStats
	prErr   error
	prDone  bool
	// focused are the authors Commits by author is limited to, by name; none means all.
	focused []string
}

type localMsg struct {
	idx  int
	data *report.Local
	err  error
}

type prMsg struct {
	idx  int
	data *remote.PRStats
	err  error
}

type Model struct {
	opts          Options
	repos         []repoState
	width, height int
	tab           int
	scroll        int
	spin          spinner.Model
	window        windowMode
	beforeOverlap windowMode   // what o returns to
	chartHeight   int          // Activity line chart rows; 0 means defaultChartH
	expanded      uint8        // bit per tab: e shows every row of that tab's lists
	prSort        prSortKey    // which column orders PR size by author; s cycles it
	fileFilter    fileFilter   // test / non-test filter on Files; f cycles it
	details       bool         // x adds the Email column to the Authors table
	authorPeriod  authorPeriod // what Commits by author counts over; p cycles it
	hover         authorRef    // whose name in a Commits by author legend the pointer is over
	linear        bool         // y draws Commits by author on a linear scale rather than log
	sideBySide    bool         // when comparing, c puts repos in columns instead of stacking them
	// content is the rendered body, split into lines. Rendering a tab can take
	// milliseconds (Activity draws several charts), so it's redone only when
	// something shown changes; scrolling just moves a window over these lines.
	content []string
	// spots are the clickable parts of content, by line; see clickable.
	spots map[int][]hotspot
	// headings are the lines of content holding repo headings, which stay
	// pinned at the top while their repo's sections scroll past.
	headings []int
	// columnsOn is whether repos are side by side right now; see fitColumns.
	columnsOn bool
	// pointer is the last cell the mouse was seen over; whatever clickable
	// thing is there is drawn highlighted. pointerSeen is false until then.
	pointerX, pointerY int
	pointerSeen        bool
}

func New(opts Options) Model {
	names := displayNames(opts.Roots)
	repos := make([]repoState, len(opts.Roots))
	for i := range repos {
		repos[i] = repoState{name: names[i], color: repoColors[i%len(repoColors)]}
	}
	return Model{
		opts:  opts,
		repos: repos,
		spin:  spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(titleStyle)),
	}
}

// displayNames uses each repo's directory name, adding the parent when two would collide.
func displayNames(roots []string) []string {
	count := map[string]int{}
	for _, r := range roots {
		count[filepath.Base(r)]++
	}
	names := make([]string, len(roots))
	for i, r := range roots {
		names[i] = filepath.Base(r)
		if count[names[i]] > 1 {
			names[i] = filepath.Join(filepath.Base(filepath.Dir(r)), names[i])
		}
	}
	return names
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spin.Tick}
	for i, root := range m.opts.Roots {
		cmds = append(cmds, loadLocal(i, root, m.opts.Since, m.opts.AllFiles))
		if !m.opts.NoRemote {
			cmds = append(cmds, loadPRs(i, root))
		}
	}
	return tea.Batch(cmds...)
}

func loadLocal(idx int, root, since string, allFiles bool) tea.Cmd {
	return func() tea.Msg {
		d, err := report.CollectLocal(context.Background(), root, since, allFiles)
		return localMsg{idx, d, err}
	}
}

func loadPRs(idx int, root string) tea.Cmd {
	return func() tea.Msg {
		d, err := remote.FetchPRs(context.Background(), root)
		return prMsg{idx, d, err}
	}
}

func (m Model) loading() bool {
	for _, r := range m.repos {
		if (r.local == nil && r.localErr == nil) || (!m.opts.NoRemote && !r.prDone) {
			return true
		}
	}
	return false
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	rerender := true
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case localMsg:
		r := &m.repos[msg.idx]
		r.local, r.localErr = msg.data, msg.err
		// Overlap depends on every repo, so recompute all of them.
		m.applyWindow()
	case prMsg:
		r := &m.repos[msg.idx]
		r.prs, r.prErr, r.prDone = msg.data, msg.err, true
	case spinner.TickMsg:
		// Loading messages in the body show the spinner, so it redraws while loading.
		if !m.loading() {
			return m, nil
		}
		m.spin, cmd = m.spin.Update(msg)
	case tea.KeyMsg:
		rerender = false
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "tab", "right", "l":
			m.setTab((m.tab + 1) % len(tabNames))
			rerender = true
		case "shift+tab", "left", "h":
			m.setTab((m.tab + len(tabNames) - 1) % len(tabNames))
			rerender = true
		case "1", "2", "3", "4", "5":
			m.setTab(int(msg.String()[0] - '1'))
			rerender = true
		case "down", "j":
			m.scroll = min(m.scroll+1, m.maxScroll())
		case "up", "k":
			m.scroll = max(m.scroll-1, 0)
		case "pgdown", " ":
			m.scroll = min(m.scroll+m.bodyHeight(), m.maxScroll())
		case "pgup":
			m.scroll = max(m.scroll-m.bodyHeight(), 0)
		case "+", "=":
			if m.hasCharts() {
				m.resizeCharts(chartStep) // renders itself
			}
		case "-", "_":
			if m.hasCharts() {
				m.resizeCharts(-chartStep)
			}
		case "e":
			m.expanded ^= 1 << m.tab
			rerender = true
		case "s":
			if m.tab == prTab {
				m.prSort = (m.prSort + 1) % prSortKeys
				rerender = true
			}
		case "c":
			if m.comparing() {
				m.sideBySide = !m.sideBySide
				rerender = true
			}
		case "f":
			if m.tab == filesTab {
				m.fileFilter = (m.fileFilter + 1) % fileFilters
				rerender = true
			}
		case "x":
			if m.tab == authorsTab {
				m.details = !m.details
				rerender = true
			}
		case "p":
			if m.tab == authorsTab {
				m.authorPeriod = (m.authorPeriod + 1) % authorPeriods
				rerender = true
			}
		case "a":
			if m.tab == authorsTab {
				m.cycleFocus()
				rerender = true
			}
		case "y":
			if m.tab == authorsTab {
				m.linear = !m.linear
				rerender = true
			}
		case "t":
			m.window = m.window.next(m.comparing())
			m.applyWindow()
			rerender = true
		case "o":
			if m.comparing() {
				if m.window == windowOverlap {
					m.window = m.beforeOverlap
				} else {
					m.beforeOverlap, m.window = m.window, windowOverlap
				}
				m.applyWindow()
				rerender = true
			}
		}
	case tea.MouseMsg:
		// A click on something with a key presses that key; the wheel scrolls.
		// Plain movement only moves the highlight, which View draws.
		m.pointerX, m.pointerY, m.pointerSeen = msg.X, msg.Y, true
		key := m.mouse(msg)
		if by, ok := strings.CutPrefix(key, sortClick); ok {
			m.prSort = prSortKey(by[0] - '0')
			break
		}
		if a, ok := m.legendAuthor(key); ok {
			m.toggleFocus(a)
			break
		}
		if key != "" {
			return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		}
		// Hovering a name in a Commits by author legend redraws the chart with
		// that author picked out.
		if a := m.hoveredAuthor(); a != m.hover {
			m.hover = a
			break
		}
		rerender = false
	default:
		rerender = false
	}
	if rerender {
		m.render()
	}
	return m, cmd
}

// render redraws the active tab into content and keeps scroll in range.
func (m *Model) render() {
	m.columnsOn = m.fitColumns()
	m.content = strings.Split(m.body(), "\n")
	m.spots = map[int][]hotspot{}
	for n, line := range m.content {
		if strings.Contains(line, markPrefix) {
			m.content[n], m.spots[n] = takeSpots(line)
		}
	}
	m.headings = m.headingLines()
	m.scroll = min(m.scroll, m.maxScroll())
}

// headingLines finds the repo headings in content: one per repo when stacked,
// a single row when side by side.
func (m Model) headingLines() []int {
	if !m.comparing() {
		return nil
	}
	var out []int
	for n, line := range m.content {
		plain := strings.TrimSpace(ansi.Strip(line))
		for _, r := range m.repos {
			if strings.HasPrefix(plain, "● "+r.name+" ─") {
				out = append(out, n)
				break
			}
		}
	}
	return out
}

// stickyHeading is the heading line to pin over the top of the body when
// scrolled to off: the last one at or above the top, or -1 when there's none
// or it's already the top line.
func (m Model) stickyHeading(off int) int {
	top := -1
	for _, n := range m.headings {
		if n > off {
			break
		}
		top = n
	}
	if top == off {
		return -1
	}
	return top
}

func (m *Model) setTab(t int) {
	m.tab = t
	m.scroll = 0
	m.hover = authorRef{}
}

func (m Model) bodyHeight() int {
	return max(m.height-lipgloss.Height(m.header())-lipgloss.Height(m.footer()), 1)
}

func (m Model) maxScroll() int {
	return max(len(m.content)-m.bodyHeight(), 0)
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	if m.width < 64 || m.height < 12 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dimStyle.Render("window too small"))
	}
	var visible []string
	for _, n := range m.visibleRows() {
		visible = append(visible, m.content[n])
	}
	if spots := m.bodySpots(m.pointerY); m.pointerSeen && len(spots) > 0 {
		row := m.pointerY - lipgloss.Height(m.header())
		visible[row] = m.highlight(visible[row], 0, spots...)
	}
	for len(visible) < m.bodyHeight() {
		visible = append(visible, "")
	}
	return m.header() + "\n" + strings.Join(visible, "\n") + "\n" + m.footer()
}

// visibleRows is the line of content each body row shows at the current
// scroll, with the sticky heading pinned over the top one.
func (m Model) visibleRows() []int {
	h := m.bodyHeight()
	off := min(m.scroll, max(len(m.content)-h, 0))
	var rows []int
	for n := off; n < min(off+h, len(m.content)); n++ {
		rows = append(rows, n)
	}
	if top := m.stickyHeading(off); top >= 0 && len(rows) > 0 {
		rows[0] = top
	}
	return rows
}

// bodySpots are the clickable parts of the body row on screen line y.
func (m Model) bodySpots(y int) []hotspot {
	rows := m.visibleRows()
	row := y - lipgloss.Height(m.header())
	if row < 0 || row >= len(rows) {
		return nil
	}
	spots := slices.Clone(m.spots[rows[row]])
	for i := range spots {
		spots[i].y = y
	}
	return spots
}

func (m Model) header() string {
	s, _ := m.headerLayout()
	return s
}

// headerLayout renders the header along with where its clickable parts are.
func (m Model) headerLayout() (string, []hotspot) {
	var spots []hotspot
	title := titleStyle.Render("repoanalyzer")
	for _, r := range m.repos {
		label := r.name
		if len(m.repos) > 1 {
			label = "● " + label
		}
		title += "  " + lipgloss.NewStyle().Foreground(r.color).Render(label)
		if r.local != nil {
			title += dimStyle.Render(" · " + r.local.Branch)
		}
	}
	if m.opts.Since != "" {
		title += dimStyle.Render("  since " + m.opts.Since)
	}
	if m.opts.AllFiles {
		title += dimStyle.Render("  all files")
	}
	var tabs []string
	x := 0
	for i, name := range tabNames {
		key := string(rune('1' + i))
		label := key + " " + name
		tab := inactiveTabStyle.Render(label)
		if i == m.tab {
			tab = activeTabStyle.Render(label)
		}
		spot := hotspot{y: 1, x: x, w: lipgloss.Width(tab), key: key}
		// The current tab doesn't light up: clicking it changes nothing you'd notice.
		if i != m.tab {
			tab = m.highlight(tab, x, spot)
		}
		tabs = append(tabs, tab)
		spots = append(spots, spot)
		x += spot.w
	}
	tabRow := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	// View settings sit at the right of the tab row, or on their own line if they won't fit.
	// Clicking one does what its key does.
	var settings, settingKeys []string
	if label := m.chartLabel(); label != "" {
		settings = append(settings, lipgloss.NewStyle().Foreground(accent).Render(label))
		settingKeys = append(settingKeys, "")
	}
	if label := m.columnsLabel(); label != "" {
		style := lipgloss.NewStyle().Foreground(accent)
		if !m.columns() {
			style = dimStyle
		}
		settings = append(settings, style.Render(label))
		settingKeys = append(settingKeys, "c")
	}
	if label := m.filterLabel(); label != "" {
		settings = append(settings, lipgloss.NewStyle().Foreground(accent).Render(label))
		settingKeys = append(settingKeys, "f")
	}
	if label := m.periodLabel(); label != "" {
		settings = append(settings, lipgloss.NewStyle().Foreground(accent).Render(label))
		settingKeys = append(settingKeys, "p")
	}
	if label := m.focusLabel(); label != "" {
		settings = append(settings, lipgloss.NewStyle().Foreground(accent).Render(label))
		settingKeys = append(settingKeys, "a")
	}
	if label := m.scaleLabel(); label != "" {
		settings = append(settings, lipgloss.NewStyle().Foreground(accent).Render(label))
		settingKeys = append(settingKeys, "y")
	}
	if label := m.windowLabel(); label != "" {
		style := lipgloss.NewStyle().Foreground(accent)
		if !m.windowed() {
			style = dimStyle
		}
		settings = append(settings, style.Render("◷ "+label))
		settingKeys = append(settingKeys, "t")
	}
	if len(settings) > 0 {
		label := strings.Join(settings, "  ") + " "
		y, x := 1, m.width-lipgloss.Width(label)
		gap := m.width - lipgloss.Width(tabRow) - lipgloss.Width(label)
		if gap < 2 {
			y, x = 2, 1
		}
		for i, s := range settings {
			spot := hotspot{y: y, x: x, w: lipgloss.Width(s), key: settingKeys[i]}
			settings[i] = m.highlight(s, x, spot)
			spots = append(spots, spot)
			x += spot.w + 2
		}
		label = strings.Join(settings, "  ") + " "
		if gap >= 2 {
			tabRow += strings.Repeat(" ", gap) + label
		} else {
			tabRow += "\n " + lipgloss.NewStyle().MaxWidth(m.width-1).Render(label)
		}
	}
	rule := dimStyle.Render(strings.Repeat("─", m.width))
	title = lipgloss.NewStyle().MaxWidth(m.width).Render(" " + title)
	return title + "\n" + tabRow + "\n" + rule, spots
}

func (m Model) footer() string {
	s, _ := m.footerLayout()
	return s
}

// footerLayout renders a hint for every control that does something right
// now, wrapping onto more lines rather than leaving any out, along with where
// each hint is, so a click on one can press its key.
func (m Model) footerLayout() (string, []hotspot) {
	type hint struct{ text, key string }
	keys := []hint{{"←/→ tabs", ""}, {"↑/↓ scroll", ""}, {"pgup/pgdn page", ""}, {"t window", "t"}}
	if m.comparing() {
		keys = append(keys, hint{"o overlap", "o"}, hint{"c columns", "c"})
	}
	if m.hasCharts() {
		keys = append(keys, hint{"+/- chart height", "+-"})
	}
	if m.tab == prTab {
		keys = append(keys, hint{"s sort PRs", "s"})
	}
	if m.tab == filesTab {
		keys = append(keys, hint{"f test filter", "f"})
	}
	if m.tab == authorsTab {
		if m.details {
			keys = append(keys, hint{"x fewer details", "x"})
		} else {
			keys = append(keys, hint{"x more details", "x"})
		}
		keys = append(keys, hint{"p period", "p"})
		if m.linear {
			keys = append(keys, hint{"y log scale", "y"})
		} else {
			keys = append(keys, hint{"y linear scale", "y"})
		}
		if m.focusable() > 0 {
			keys = append(keys, hint{"a focus author", "a"})
		}
	}
	switch {
	case !m.expandable():
	case m.showAll():
		keys = append(keys, hint{"e show fewer", "e"})
	default:
		keys = append(keys, hint{"e show all", "e"})
	}
	keys = append(keys, hint{"q quit", "q"})
	prefix := " "
	if m.loading() {
		prefix = " " + m.spin.View() + " loading… · "
	}

	// Lay the hints out first, then place the lines at the bottom of the screen.
	sep := " · "
	type placed struct {
		hint
		line, x int
	}
	var hints []placed
	line, x := 0, lipgloss.Width(prefix)
	for i, k := range keys {
		if i > 0 {
			x += lipgloss.Width(sep)
		}
		if w := lipgloss.Width(k.text); i > 0 && x+w > m.width {
			line, x = line+1, 1
		}
		hints = append(hints, placed{k, line, x})
		x += lipgloss.Width(k.text)
	}
	top := m.height - (line + 1)

	// A one-key hint is clickable all over; with several keys, each is
	// clickable where it appears in the text.
	var spots []hotspot
	lines := make([]string, line+1)
	lines[0] = dimStyle.Render(prefix)
	for i, h := range hints {
		y := top + h.line
		var own []hotspot
		if len(h.key) == 1 {
			own = append(own, hotspot{y: y, x: h.x, w: lipgloss.Width(h.text), key: h.key})
		} else {
			for _, r := range h.key {
				at := strings.IndexRune(h.text, r)
				own = append(own, hotspot{y: y, x: h.x + lipgloss.Width(h.text[:at]), w: 1, key: string(r)})
			}
		}
		switch {
		case i == 0:
		case h.line == hints[i-1].line:
			lines[h.line] += dimStyle.Render(sep)
		default:
			lines[h.line] = " "
		}
		lines[h.line] += m.highlight(dimStyle.Render(h.text), h.x, own...)
		spots = append(spots, own...)
	}
	return lipgloss.NewStyle().MaxWidth(m.width).Render(strings.Join(lines, "\n")), spots
}

// body renders the active tab, padded one column from the left edge.
func (m Model) body() string {
	w := max(m.width-2, 20)
	var s string
	switch m.tab {
	case 0:
		s = m.overview(w)
	case 1:
		s = m.authors(w)
	case activityTab:
		s = m.activity(w)
	case filesTab:
		s = m.each(w, m.filesRepo)
	case prTab:
		s = m.pullRequests(w)
	}
	return lipgloss.NewStyle().PaddingLeft(1).Render(s)
}

func (m Model) comparing() bool {
	return len(m.repos) > 1
}

// status returns a loading or error line for a repo whose local data isn't usable yet, else "".
func (m Model) status(r *repoState, w int) string {
	switch {
	case r.localErr != nil:
		return errStyle.Width(w).Render("Error: " + r.localErr.Error())
	case r.local == nil:
		return m.spin.View() + " Reading files and git history…"
	}
	return ""
}

// each renders fn's sections for every repo under colored headings when comparing:
// stacked, or side by side in columns when c has turned that on and they fit.
// fn returns the same sections in the same order for every repo, so that in
// columns each one can start on the same line everywhere.
func (m Model) each(w int, fn func(r *repoState, w int) []string) string {
	colW := w
	if m.columns() {
		colW = m.columnWidth()
	}
	cols := make([][]string, len(m.repos))
	for i := range m.repos {
		r := &m.repos[i]
		if s := m.status(r, colW); s != "" {
			cols[i] = []string{s}
		} else {
			cols[i] = fn(r, colW)
		}
		if m.comparing() {
			cols[i] = append([]string{repoHeading(r, colW)}, cols[i]...)
		}
	}
	if !m.columns() {
		var parts []string
		for _, sections := range cols {
			parts = append(parts, strings.Join(sections, "\n"))
		}
		return strings.Join(parts, "\n\n")
	}

	// Pad every section to its tallest version across repos (a missing one is all
	// padding), so a longer list in one repo pushes the next section down in all of them.
	n := 0
	for _, sections := range cols {
		n = max(n, len(sections))
	}
	parts := make([]string, len(cols))
	for k := range n {
		h := 0
		for _, sections := range cols {
			if k < len(sections) {
				h = max(h, lipgloss.Height(sections[k]))
			}
		}
		for i, sections := range cols {
			s := ""
			if k < len(sections) {
				s = sections[k]
			}
			s += strings.Repeat("\n", h-lipgloss.Height(s))
			if k > 0 {
				s = "\n" + s
			}
			parts[i] += s
		}
	}
	for i, p := range parts {
		// Cut anything that overflows, then pad, so every column starts at the same place.
		p = lipgloss.NewStyle().MaxWidth(colW).Render(p)
		parts[i] = lipgloss.NewStyle().Width(colW).Render(p)
		if i < len(parts)-1 {
			parts[i] = lipgloss.NewStyle().MarginRight(columnGap).Render(parts[i])
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// oneSection adapts a renderer that draws a single section for each.
func oneSection(fn func(r *repoState, w int) string) func(r *repoState, w int) []string {
	return func(r *repoState, w int) []string { return []string{fn(r, w)} }
}

// Side-by-side columns are this far apart and need at least minColumnW each.
const (
	columnGap  = 3
	minColumnW = 50
)

// columnWidth is each repo's column width when side by side.
func (m Model) columnWidth() int {
	n := len(m.repos)
	return (max(m.width-2, 20) - columnGap*(n-1)) / n
}

// columns reports whether repos are shown side by side right now.
func (m Model) columns() bool {
	return m.columnsOn
}

// fitColumns decides whether repos go side by side: c has to have turned it on,
// and the columns have to be wide enough that the current tab doesn't need
// squeezing. Otherwise they stack.
func (m Model) fitColumns() bool {
	if !m.sideBySide || !m.comparing() || m.columnWidth() < minColumnW {
		return false
	}
	if m.tab == authorsTab {
		_, fits := m.authorColumns(m.columnWidth())
		return fits
	}
	return true
}

// columnsLabel shows the side-by-side setting in the header while it's on.
func (m Model) columnsLabel() string {
	switch {
	case !m.sideBySide || !m.comparing():
		return ""
	case !m.columns():
		return "▥ side by side: too narrow, stacked"
	}
	return "▥ side by side"
}

func repoHeading(r *repoState, w int) string {
	label := lipgloss.NewStyle().Bold(true).Foreground(r.color).Render("● " + r.name)
	rule := strings.Repeat("─", max(w-lipgloss.Width(label)-1, 0))
	return label + " " + lipgloss.NewStyle().Foreground(r.color).Faint(true).Render(rule)
}
