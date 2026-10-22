// Package ui is the Bubble Tea dashboard.
package ui

import (
	"context"
	"path/filepath"
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
}

var tabNames = []string{"Overview", "Authors", "Activity", "Files", "Pull Requests"}

const (
	authorsTab  = 1
	activityTab = 2
	filesTab    = 3
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
	beforeOverlap windowMode // what o returns to
	chartHeight   int        // Activity line chart rows; 0 means defaultChartH
	expanded      uint8      // bit per tab: e shows every row of that tab's lists
	prSort        prSortKey  // which column orders PR size by author; s cycles it
	fileFilter    fileFilter // test / non-test filter on Files; f cycles it
	sideBySide    bool       // when comparing, c puts repos in columns instead of stacking them
	// content is the rendered body, split into lines. Rendering a tab can take
	// milliseconds (Activity draws several charts), so it's redone only when
	// something shown changes; scrolling just moves a window over these lines.
	content []string
	// headings are the lines of content holding repo headings, which stay
	// pinned at the top while their repo's sections scroll past.
	headings []int
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
		cmds = append(cmds, loadLocal(i, root, m.opts.Since))
		if !m.opts.NoRemote {
			cmds = append(cmds, loadPRs(i, root))
		}
	}
	return tea.Batch(cmds...)
}

func loadLocal(idx int, root, since string) tea.Cmd {
	return func() tea.Msg {
		d, err := report.CollectLocal(context.Background(), root, since)
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
			if m.tab == activityTab {
				m.resizeCharts(chartStep) // renders itself
			}
		case "-", "_":
			if m.tab == activityTab {
				m.resizeCharts(-chartStep)
			}
		case "e":
			m.expanded ^= 1 << m.tab
			rerender = true
		case "s":
			if m.tab == authorsTab {
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
	m.content = strings.Split(m.body(), "\n")
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
	lines := m.content
	h := m.bodyHeight()
	off := min(m.scroll, max(len(lines)-h, 0))
	visible := lines[off:min(off+h, len(lines))]
	if top := m.stickyHeading(off); top >= 0 && len(visible) > 0 {
		visible = append([]string{lines[top]}, visible[1:]...)
	}
	for len(visible) < h {
		visible = append(visible, "")
	}
	return m.header() + "\n" + strings.Join(visible, "\n") + "\n" + m.footer()
}

func (m Model) header() string {
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
	var tabs []string
	for i, name := range tabNames {
		label := string(rune('1'+i)) + " " + name
		if i == m.tab {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}
	tabRow := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	// View settings sit at the right of the tab row, or on their own line if they won't fit.
	var settings []string
	if label := m.chartLabel(); label != "" {
		settings = append(settings, lipgloss.NewStyle().Foreground(accent).Render(label))
	}
	if label := m.columnsLabel(); label != "" {
		style := lipgloss.NewStyle().Foreground(accent)
		if !m.columns() {
			style = dimStyle
		}
		settings = append(settings, style.Render(label))
	}
	if label := m.filterLabel(); label != "" {
		settings = append(settings, lipgloss.NewStyle().Foreground(accent).Render(label))
	}
	if label := m.windowLabel(); label != "" {
		style := lipgloss.NewStyle().Foreground(accent)
		if !m.windowed() {
			style = dimStyle
		}
		settings = append(settings, style.Render("◷ "+label))
	}
	if len(settings) > 0 {
		label := strings.Join(settings, "  ") + " "
		if gap := m.width - lipgloss.Width(tabRow) - lipgloss.Width(label); gap >= 2 {
			tabRow += strings.Repeat(" ", gap) + label
		} else {
			tabRow += "\n " + lipgloss.NewStyle().MaxWidth(m.width-1).Render(label)
		}
	}
	rule := dimStyle.Render(strings.Repeat("─", m.width))
	title = lipgloss.NewStyle().MaxWidth(m.width).Render(" " + title)
	return title + "\n" + tabRow + "\n" + rule
}

func (m Model) footer() string {
	keys := []string{"←/→ tabs", "↑/↓ scroll", "t window"}
	if m.comparing() {
		keys = append(keys, "o overlap", "c columns")
	}
	if m.tab == activityTab {
		keys = append(keys, "+/- chart height")
	}
	if m.tab == authorsTab {
		keys = append(keys, "s sort PRs")
	}
	if m.tab == filesTab {
		keys = append(keys, "f test filter")
	}
	keys = append(keys, "q quit")
	prefix := " "
	if m.loading() {
		prefix = " " + m.spin.View() + " loading… · "
	}
	// Drop the navigation hints first if everything won't fit.
	help := prefix + strings.Join(keys, " · ")
	for len(keys) > 4 && lipgloss.Width(help) > m.width {
		keys = keys[1:]
		help = prefix + strings.Join(keys, " · ")
	}
	return dimStyle.MaxWidth(m.width).Render(help)
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
	case 4:
		s = m.each(w, m.pullRequestsRepo)
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
	return m.sideBySide && m.comparing() && m.columnWidth() >= minColumnW
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
