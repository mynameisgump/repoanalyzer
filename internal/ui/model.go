// Package ui is the Bubble Tea dashboard.
package ui

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ethancrann/repoanalyzer/internal/remote"
	"github.com/ethancrann/repoanalyzer/internal/report"
)

// MaxRepos is how many repos can be compared at once (one color each).
var MaxRepos = len(repoColors)

type Options struct {
	Roots    []string // repo top-level directories; more than one compares them
	Since    string
	NoRemote bool
}

var tabNames = []string{"Overview", "Authors", "Activity", "Files", "Pull Requests"}

// repoState is one repo's data as it arrives.
type repoState struct {
	name     string
	color    lipgloss.AdaptiveColor
	local    *report.Local
	localErr error
	prs      *remote.PRStats
	prErr    error
	prDone   bool
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
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case localMsg:
		r := &m.repos[msg.idx]
		r.local, r.localErr = msg.data, msg.err
	case prMsg:
		r := &m.repos[msg.idx]
		r.prs, r.prErr, r.prDone = msg.data, msg.err, true
	case spinner.TickMsg:
		if m.loading() {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "tab", "right", "l":
			m.setTab((m.tab + 1) % len(tabNames))
		case "shift+tab", "left", "h":
			m.setTab((m.tab + len(tabNames) - 1) % len(tabNames))
		case "1", "2", "3", "4", "5":
			m.setTab(int(msg.String()[0] - '1'))
		case "down", "j":
			m.scroll = min(m.scroll+1, m.maxScroll())
		case "up", "k":
			m.scroll = max(m.scroll-1, 0)
		case "pgdown", " ":
			m.scroll = min(m.scroll+m.bodyHeight(), m.maxScroll())
		case "pgup":
			m.scroll = max(m.scroll-m.bodyHeight(), 0)
		}
	}
	return m, nil
}

func (m *Model) setTab(t int) {
	m.tab = t
	m.scroll = 0
}

func (m Model) bodyHeight() int {
	return max(m.height-lipgloss.Height(m.header())-lipgloss.Height(m.footer()), 1)
}

func (m Model) maxScroll() int {
	lines := strings.Count(m.body(), "\n") + 1
	return max(lines-m.bodyHeight(), 0)
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	if m.width < 64 || m.height < 12 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dimStyle.Render("window too small"))
	}
	lines := strings.Split(m.body(), "\n")
	h := m.bodyHeight()
	off := min(m.scroll, max(len(lines)-h, 0))
	visible := lines[off:min(off+h, len(lines))]
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
	rule := dimStyle.Render(strings.Repeat("─", m.width))
	title = lipgloss.NewStyle().MaxWidth(m.width).Render(" " + title)
	return title + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, tabs...) + "\n" + rule
}

func (m Model) footer() string {
	help := "←/→ tabs · ↑/↓ scroll · q quit"
	if m.loading() {
		help = m.spin.View() + " loading… · " + help
	}
	return dimStyle.Render(" " + help)
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
	case 2:
		s = m.activity(w)
	case 3:
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

// each renders fn for every repo, stacked under colored headings when comparing.
func (m Model) each(w int, fn func(r *repoState, w int) string) string {
	var parts []string
	for i := range m.repos {
		r := &m.repos[i]
		s := m.status(r, w)
		if s == "" {
			s = fn(r, w)
		}
		if m.comparing() {
			s = repoHeading(r, w) + "\n" + s
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n\n")
}

func repoHeading(r *repoState, w int) string {
	label := lipgloss.NewStyle().Bold(true).Foreground(r.color).Render("● " + r.name)
	rule := strings.Repeat("─", max(w-lipgloss.Width(label)-1, 0))
	return label + " " + lipgloss.NewStyle().Foreground(r.color).Faint(true).Render(rule)
}
