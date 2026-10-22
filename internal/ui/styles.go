package ui

import "github.com/charmbracelet/lipgloss"

var (
	accent = lipgloss.AdaptiveColor{Light: "#5A4FCF", Dark: "#A496FF"}
	subtle = lipgloss.AdaptiveColor{Light: "#8C8C8C", Dark: "#6E6E6E"}
	green  = lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#3FB950"}
	red    = lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#F85149"}

	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	dimStyle     = lipgloss.NewStyle().Foreground(subtle)
	boldStyle    = lipgloss.NewStyle().Bold(true)
	addStyle     = lipgloss.NewStyle().Foreground(green)
	delStyle     = lipgloss.NewStyle().Foreground(red)
	errStyle     = lipgloss.NewStyle().Foreground(red)
	sectionStyle = lipgloss.NewStyle().Bold(true).MarginTop(1).MarginBottom(1)

	activeTabStyle   = lipgloss.NewStyle().Bold(true).Foreground(accent).Underline(true).Padding(0, 1)
	inactiveTabStyle = lipgloss.NewStyle().Foreground(subtle).Padding(0, 1)

	tileStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Padding(0, 1).
			MarginRight(1)
	tileValueStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)

	// Distinct hues for language / author bars, cycled in order.
	palette = []lipgloss.AdaptiveColor{
		{Light: "#5A4FCF", Dark: "#A496FF"},
		{Light: "#0969DA", Dark: "#58A6FF"},
		{Light: "#1A7F37", Dark: "#3FB950"},
		{Light: "#9A6700", Dark: "#D29922"},
		{Light: "#BC4C00", Dark: "#F0883E"},
		{Light: "#CF222E", Dark: "#F85149"},
		{Light: "#BF3989", Dark: "#DB61A2"},
		{Light: "#1B7C83", Dark: "#39C5CF"},
	}

	// One color per compared repo; the first matches the single-repo accent.
	repoColors = []lipgloss.AdaptiveColor{
		{Light: "#5A4FCF", Dark: "#A496FF"}, // purple
		{Light: "#BC4C00", Dark: "#F0883E"}, // orange
		{Light: "#1B7C83", Dark: "#39C5CF"}, // teal
		{Light: "#BF3989", Dark: "#DB61A2"}, // pink
	}

	// Activity heatmap intensity, from empty to busiest.
	heatRamp = []lipgloss.AdaptiveColor{
		{Light: "#EBEDF0", Dark: "#2D333B"},
		{Light: "#9BE9A8", Dark: "#0E4429"},
		{Light: "#40C463", Dark: "#006D32"},
		{Light: "#30A14E", Dark: "#26A641"},
		{Light: "#216E39", Dark: "#39D353"},
	}
	// Shading doubles up on color so the heatmap still reads without color support.
	heatGlyphs = []string{"·", "░", "▒", "▓", "█"}
)
