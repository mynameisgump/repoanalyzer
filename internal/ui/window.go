package ui

import (
	"time"

	"github.com/ethancrann/repoanalyzer/internal/stats"
)

// windowMode limits history stats to a span of time; t cycles through them, o toggles overlap.
type windowMode int

const (
	windowAll windowMode = iota
	windowYear
	windowHalf
	windowQuarter
	windowOverlap // only while comparing: the span every repo has history for
)

func (w windowMode) label() string {
	return [...]string{"all time", "last year", "last 6 months", "last 3 months", "overlap"}[w]
}

// next is the mode t switches to; overlap is skipped for a single repo.
func (w windowMode) next(comparing bool) windowMode {
	n := (w + 1) % (windowOverlap + 1)
	if n == windowOverlap && !comparing {
		n = windowAll
	}
	return n
}

// span is the window's time range. ok is false when the window can't apply,
// in which case why says so and all-time stats are shown instead.
func (m Model) span() (from, to time.Time, ok bool, why string) {
	now := time.Now()
	switch m.window {
	case windowAll:
		return time.Time{}, time.Time{}, false, ""
	case windowYear:
		return now.AddDate(-1, 0, 0), now, true, ""
	case windowHalf:
		return now.AddDate(0, -6, 0), now, true, ""
	case windowQuarter:
		return now.AddDate(0, -3, 0), now, true, ""
	}

	// Overlap: the latest first change to the earliest last change, over the repos that loaded.
	loaded := 0
	for _, r := range m.repos {
		switch {
		case r.localErr != nil:
			continue
		case r.local == nil:
			return time.Time{}, time.Time{}, false, "waiting for every repo to load"
		}
		first, last := r.local.Span()
		if first.IsZero() {
			continue
		}
		if loaded == 0 || first.After(from) {
			from = first
		}
		if loaded == 0 || last.Before(to) {
			to = last
		}
		loaded++
	}
	switch {
	case loaded < 2:
		return time.Time{}, time.Time{}, false, "needs two repos with history"
	case from.After(to):
		return time.Time{}, time.Time{}, false, "these histories don't overlap"
	}
	return from, to, true, ""
}

// applyWindow points every loaded repo's hist at the stats for the current window,
// computing and caching them on first use.
func (m *Model) applyWindow() {
	from, to, ok, _ := m.span()
	for i := range m.repos {
		r := &m.repos[i]
		if r.local == nil {
			continue
		}
		if !ok {
			r.hist = r.local.History
			continue
		}
		if r.windows == nil {
			r.windows = map[windowMode]stats.Summary{}
		}
		s, cached := r.windows[m.window]
		if !cached {
			s = r.local.InWindow(from, to)
			r.windows[m.window] = s
		}
		r.hist = s
	}
	m.scroll = min(m.scroll, m.maxScroll())
}

// windowed reports whether history stats are currently limited to a time window.
func (m Model) windowed() bool {
	_, _, ok, _ := m.span()
	return ok
}

// treeNote marks sections that show the current tree, which a time window doesn't change.
func (m Model) treeNote() string {
	if m.windowed() {
		return " · current tree"
	}
	return ""
}

// windowLabel describes the active window for the header, or "" for all time.
func (m Model) windowLabel() string {
	if m.window == windowAll {
		return ""
	}
	from, to, ok, why := m.span()
	if !ok {
		return m.window.label() + ": " + why + ", showing all time"
	}
	return m.window.label() + " · " + from.Format("Jan 2, 2006") + " – " + to.Format("Jan 2, 2006")
}
