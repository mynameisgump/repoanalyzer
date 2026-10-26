package ui

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// authorPeriod is what Commits by author counts over; p cycles it.
type authorPeriod int

const (
	perTwoWeeks authorPeriod = iota // the default
	perMonth
	perWeek
	authorPeriods // number of periods
)

func (p authorPeriod) String() string {
	return [...]string{"2 weeks", "month", "week"}[p]
}

// fortnightStart is the start of the two weeks holding Monday wk. Fortnights
// are counted from a fixed Monday, so they don't shift with the time window.
func fortnightStart(wk time.Time) time.Time {
	epoch := time.Date(1970, 1, 5, 0, 0, 0, 0, time.Local)
	days := int(math.Round(wk.Sub(epoch).Hours() / 24))
	return epoch.AddDate(0, 0, days/14*14)
}

// focusClick, then an author's rank from 1, ":" and their repo's name, adds
// that author to the ones Commits by author shows, or takes them out again.
const focusClick = "focus:"

// authorRef is an author in one repo's Commits by author; a zero one is no one.
type authorRef struct {
	repo, name string
}

// chartedAuthors are the authors Commits by author has a line for: the top ones.
func chartedAuthors(r *repoState) []string {
	var names []string
	for _, a := range r.hist.Authors[:min(len(r.hist.Authors), topAuthorsCharted)] {
		names = append(names, a.Name)
	}
	return names
}

// shownFocus is which of r's focused authors are charted in the current window.
func shownFocus(r *repoState) []string {
	charted := chartedAuthors(r)
	return slices.DeleteFunc(slices.Clone(r.focused), func(name string) bool { return !slices.Contains(charted, name) })
}

// commitsByAuthorChart overlays r's top authors' commits per period, one color
// each. Picking some out (by clicking their names in the legend) leaves only
// theirs, and hovering a name draws that author over the rest, greyed out.
func (m Model) commitsByAuthorChart(r *repoState, w int, b chartBounds) string {
	h := r.hist
	periods := h.AuthorsWeekly
	if m.authorPeriod == perMonth {
		periods = h.AuthorsMonthly
	}
	focused := shownFocus(r)
	hover := ""
	if m.hover.repo == r.name {
		hover = m.hover.name
	}
	var ss, drawn []series
	var hovered *series
	for i, name := range chartedAuthors(r) {
		s := series{name: name, color: palette[i%len(palette)]}
		for _, p := range periods {
			start, n := p.Start, float64(p.Commits(name))
			if m.authorPeriod == perTwoWeeks {
				start = fortnightStart(start)
			}
			if k := len(s.times); k > 0 && s.times[k-1].Equal(start) {
				s.vals[k-1] += n
				continue
			}
			s.times = append(s.times, start)
			s.vals = append(s.vals, n)
		}
		// Plotted on a log scale: one burst would otherwise set the y axis
		// and squash everyone's usual weeks into the bottom rows, where
		// the lines run over each other.
		for k, v := range s.vals {
			s.vals[k] = math.Log1p(v)
		}
		ss = append(ss, s)
		switch {
		case name == hover:
			s.over = true
			hovered = &s
		case len(focused) == 0 || slices.Contains(focused, name):
			if hover != "" {
				s.color = subtle
			}
			drawn = append(drawn, s)
		}
	}
	if hovered != nil { // drawn last, over the others
		drawn = append(drawn, *hovered)
	}
	commits := func(v float64) string { return num(int(math.Round(math.Expm1(v)))) }
	chart := blockChart(drawn, w, m.chartH(), commits, b)
	if len(ss) == 0 || len(ss[0].times) < 2 {
		return chart // says there isn't enough history
	}
	var entries []string
	for i, s := range ss {
		var entry string
		switch {
		case s.name == hover:
			entry = lipgloss.NewStyle().Foreground(s.color).Bold(true).Render("━━ " + s.name)
		case len(focused) > 0 && !slices.Contains(focused, s.name):
			entry = dimStyle.Render("╌╌ " + s.name) // not on the chart
		case hover != "":
			entry = dimStyle.Render("━━ " + s.name)
		default:
			entry = lipgloss.NewStyle().Foreground(s.color).Render("━━ " + s.name)
		}
		entry = clickable(focusClick+strconv.Itoa(i+1)+":"+r.name, entry)
		if i < len(ss)-1 {
			entry += "   "
		}
		entries = append(entries, entry)
	}
	return flow(entries, w) + "\n" + chart
}

// legendAuthor is who a focusClick key names, if they're still charted.
func (m Model) legendAuthor(key string) (authorRef, bool) {
	ref, ok := strings.CutPrefix(key, focusClick)
	rank, repo, _ := strings.Cut(ref, ":")
	n, err := strconv.Atoi(rank)
	if !ok || err != nil {
		return authorRef{}, false
	}
	for i := range m.repos {
		r := &m.repos[i]
		if r.name == repo && r.local != nil && r.localErr == nil {
			if charted := chartedAuthors(r); n >= 1 && n <= len(charted) {
				return authorRef{repo: repo, name: charted[n-1]}, true
			}
		}
	}
	return authorRef{}, false
}

// hoveredAuthor is the author whose legend entry is under the pointer, if any.
func (m Model) hoveredAuthor() authorRef {
	if m.tab != authorsTab || !m.pointerSeen {
		return authorRef{}
	}
	for _, s := range m.bodySpots(m.pointerY) {
		if s.hit(m.pointerX, m.pointerY) {
			if ref, ok := m.legendAuthor(s.key); ok {
				return ref
			}
		}
	}
	return authorRef{}
}

// toggleFocus adds an author to the ones their repo's chart shows, or takes them out.
func (m *Model) toggleFocus(a authorRef) {
	for i := range m.repos {
		r := &m.repos[i]
		if r.name != a.repo {
			continue
		}
		if slices.Contains(r.focused, a.name) {
			r.focused = slices.DeleteFunc(slices.Clone(r.focused), func(n string) bool { return n == a.name })
		} else {
			r.focused = append(slices.Clone(r.focused), a.name)
		}
	}
}

// cycleFocus is a: it shows every repo's top author alone, then their second,
// and so on, then everyone again. From several picked out, it shows everyone.
func (m *Model) cycleFocus() {
	for i := range m.repos {
		r := &m.repos[i]
		if r.local == nil || r.localErr != nil {
			continue
		}
		charted, focused := chartedAuthors(r), shownFocus(r)
		r.focused = nil
		switch {
		case len(charted) == 0:
		case len(focused) == 0:
			r.focused = charted[:1]
		case len(focused) == 1:
			if next := slices.Index(charted, focused[0]) + 1; next < len(charted) {
				r.focused = charted[next : next+1]
			}
		}
	}
}

// focusable is how many top authors a can cycle through: the most Commits by
// author shows for any repo.
func (m Model) focusable() int {
	n := 0
	for i := range m.repos {
		if r := &m.repos[i]; r.local != nil && r.localErr == nil {
			n = max(n, len(chartedAuthors(r)))
		}
	}
	return n
}

// periodLabel shows a non-default period for Commits by author in the header.
func (m Model) periodLabel() string {
	if m.tab != authorsTab || m.authorPeriod == perTwoWeeks {
		return ""
	}
	return "per " + m.authorPeriod.String()
}

// focusLabel names the authors Commits by author is limited to, for the
// header; comparing, it just counts them.
func (m Model) focusLabel() string {
	if m.tab != authorsTab {
		return ""
	}
	var names []string
	for i := range m.repos {
		if r := &m.repos[i]; r.local != nil && r.localErr == nil {
			names = append(names, shownFocus(r)...)
		}
	}
	switch {
	case len(names) == 0:
		return ""
	case m.comparing():
		return fmt.Sprintf("◎ %d authors", len(names))
	}
	return "◎ " + truncate(strings.Join(names, ", "), 32)
}
