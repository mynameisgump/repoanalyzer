package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// hbar draws a horizontal bar with 1/8-cell precision.
func hbar(value, peak float64, width int) string {
	if peak <= 0 || width <= 0 || value <= 0 {
		return ""
	}
	eighths := int(math.Round(value / peak * float64(width*8)))
	if eighths == 0 {
		eighths = 1 // keep tiny-but-nonzero values visible
	}
	partials := []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}
	return strings.Repeat("█", eighths/8) + partials[eighths%8]
}

type barRow struct {
	label string
	value float64
	text  string // shown after the bar, e.g. "12,340  41%"
}

// barChart renders labeled horizontal bars sized to fit width.
func barChart(rows []barRow, width int) string {
	if len(rows) == 0 {
		return dimStyle.Render("No data.")
	}
	labelW, textW, peak := 0, 0, 0.0
	for _, r := range rows {
		labelW = max(labelW, lipgloss.Width(r.label))
		textW = max(textW, lipgloss.Width(r.text))
		peak = math.Max(peak, r.value)
	}
	labelW = min(labelW, 24)
	barW := max(width-labelW-textW-4, 4)

	var b strings.Builder
	for i, r := range rows {
		color := palette[i%len(palette)]
		label := truncate(r.label, labelW)
		fmt.Fprintf(&b, "%s  %s %s\n",
			lipgloss.NewStyle().Width(labelW).Render(label),
			lipgloss.NewStyle().Foreground(color).Width(barW).Render(hbar(r.value, peak, barW)),
			dimStyle.Render(r.text))
	}
	return strings.TrimRight(b.String(), "\n")
}

func tile(label, value string) string {
	return tileStyle.Render(dimStyle.Render(label) + "\n" + tileValueStyle.Render(value))
}

// flow lays blocks out left to right, wrapping to a new row when width runs out.
func flow(blocks []string, width int) string {
	var rows []string
	var cur []string
	curW := 0
	for _, b := range blocks {
		w := lipgloss.Width(b)
		if curW+w > width && len(cur) > 0 {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cur...))
			cur, curW = nil, 0
		}
		cur = append(cur, b)
		curW += w
	}
	if len(cur) > 0 {
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cur...))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func truncate(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > w {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// truncateLeft keeps the end of long paths, which is usually the informative part.
func truncateLeft(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > w {
		r = r[1:]
	}
	return "…" + string(r)
}

// num formats an int with thousands separators.
func num(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		return "-" + s
	}
	return s
}

// shortNum abbreviates large values for axis labels: 950, 12k, 1.4M.
func shortNum(v float64) string {
	switch {
	case v >= 1e6:
		return strconv.FormatFloat(v/1e6, 'f', 1, 64) + "M"
	case v >= 1e4:
		return strconv.FormatFloat(v/1e3, 'f', 0, 64) + "k"
	case v >= 1e3:
		return strconv.FormatFloat(v/1e3, 'f', 1, 64) + "k"
	default:
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
}

func pct(part, total int) string {
	if total == 0 {
		return "0%"
	}
	p := float64(part) / float64(total) * 100
	if p < 10 {
		return fmt.Sprintf("%.1f%%", p)
	}
	return fmt.Sprintf("%.0f%%", p)
}

// humanSpan renders a coarse duration like "3y 2mo", "5mo", "12d".
func humanSpan(d time.Duration) string {
	days := int(d.Hours() / 24)
	switch {
	case days >= 365:
		y, mo := days/365, min((days%365)/30, 11)
		if mo == 0 {
			return fmt.Sprintf("%dy", y)
		}
		return fmt.Sprintf("%dy %dmo", y, mo)
	case days >= 60:
		return fmt.Sprintf("%dmo", days/30)
	case days >= 1:
		return fmt.Sprintf("%dd", days)
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
}

func ago(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return humanSpan(time.Since(t)) + " ago"
}
