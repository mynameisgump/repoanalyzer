package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ethancrann/repoanalyzer/internal/gitlog"
	"github.com/ethancrann/repoanalyzer/internal/loc"
	"github.com/ethancrann/repoanalyzer/internal/report"
	"github.com/ethancrann/repoanalyzer/internal/stats"
)

func sampleLocal() *report.Local {
	now := time.Now()
	var commits []gitlog.Commit
	for i := range 60 {
		commits = append(commits, gitlog.Commit{
			AuthorName: []string{"Ada", "Bob", "A very long author name indeed"}[i%3],
			When:       now.AddDate(0, 0, -i*5),
			Subject:    "change (#1)",
			Files:      []gitlog.FileChange{{Path: "some/really/long/path/to/a/file.go", Added: i, Deleted: 1}},
		})
	}
	h := stats.Compute(commits, now, nil)
	h.LinesOverTime = stats.LinesOverTime(commits, 25000, nil)
	h.PRSizes = stats.PRSizes(commits, nil, nil)
	return &report.Local{
		Repo: "demo", Branch: "main",
		LOC: loc.Result{Files: 3, Code: 100, TestFiles: 1, TestCode: 30,
			Languages: []loc.Language{{Name: "Go", Files: 2, Code: 90, TestFiles: 1, TestCode: 30}, {Name: "YAML", Files: 1, Code: 10}},
			Largest:   []loc.File{{Path: "main.go", Lines: 90}}},
		History: h,
	}
}

// Every tab should render at any terminal size without panicking or overflowing.
func TestRenderAllTabs(t *testing.T) {
	type repo struct {
		local  *report.Local
		err    error
		loaded bool
	}
	cases := []struct {
		name  string
		repos []repo
	}{
		{"sample", []repo{{local: sampleLocal(), loaded: true}}},
		{"empty repo", []repo{{local: &report.Local{Repo: "empty", Branch: "main"}, loaded: true}}},
		{"compare", []repo{{local: sampleLocal(), loaded: true}, {local: sampleLocal(), loaded: true}}},
		{"compare, one loading, one failed", []repo{
			{local: sampleLocal(), loaded: true}, {}, {err: errors.New("boom"), loaded: true},
		}},
	}
	for _, c := range cases {
		var roots []string
		for i := range c.repos {
			roots = append(roots, fmt.Sprintf("/src/repo%d", i))
		}
		for _, size := range [][2]int{{120, 40}, {80, 24}, {64, 12}, {63, 30}, {10, 3}} {
			var m tea.Model = New(Options{Roots: roots})
			m, _ = m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			for i, r := range c.repos {
				if r.loaded {
					m, _ = m.Update(localMsg{idx: i, data: r.local, err: r.err})
					m, _ = m.Update(prMsg{idx: i, err: errors.New("no remote")})
				}
			}
			for i := range tabNames {
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{rune('1' + i)}})
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
				out := m.View()
				if got := strings.Count(out, "\n") + 1; got != size[1] {
					t.Errorf("%s %v tab %d: view is %d lines, want %d", c.name, size, i+1, got, size[1])
				}
				for _, line := range strings.Split(out, "\n") {
					if w := lipgloss.Width(line); w > size[0] && size[0] >= 64 {
						t.Errorf("%s %v tab %d: line is %d wide: %q", c.name, size, i+1, w, line)
						break
					}
				}
			}
		}
	}
}

func TestDisplayNames(t *testing.T) {
	got := displayNames([]string{"/a/app", "/b/app", "/c/tool"})
	want := []string{"a/app", "b/app", "tool"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("displayNames = %v, want %v", got, want)
			break
		}
	}
}
