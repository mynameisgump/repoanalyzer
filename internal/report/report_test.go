package report

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Files that aren't code are left out of every stat unless allFiles is set.
func TestCollectLocalSkipsNonCode(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Ada", "GIT_AUTHOR_EMAIL=ada@example.com",
			"GIT_COMMITTER_NAME=Ada", "GIT_COMMITTER_EMAIL=ada@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	write("main.go", "package main\n\nfunc main() {}\n")   // 3 lines
	write("data.json", "[\n1,\n2,\n3\n]\n")                // 5 lines
	write("level.tscn", "[gd_scene]\n[node name=\"A\"]\n") // 2 lines
	write("README.md", "# hi\n")                           // 1 line
	git("add", ".")
	git("commit", "-q", "-m", "init")

	code, err := CollectLocal(context.Background(), dir, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if code.LOC.Files != 1 || code.LOC.Lines != 3 || code.NonCode != 3 {
		t.Errorf("code only: %d files, %d lines, %d left out; want 1, 3, 3", code.LOC.Files, code.LOC.Lines, code.NonCode)
	}
	if h := code.History; h.Commits != 1 || h.Added != 3 || len(h.HotFiles) != 1 || h.Authors[0].Added != 3 {
		t.Errorf("code only history: %d commits, %d added, %d hot files", h.Commits, h.Added, len(h.HotFiles))
	}
	if p := code.History.LinesOverTime; len(p) == 0 || p[len(p)-1].Lines != 3 {
		t.Errorf("code only lines over time = %+v", p)
	}

	all, err := CollectLocal(context.Background(), dir, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if all.LOC.Files != 4 || all.LOC.Lines != 11 || all.NonCode != 0 || all.History.Added != 11 {
		t.Errorf("all files: %d files, %d lines, %d left out, %d added", all.LOC.Files, all.LOC.Lines, all.NonCode, all.History.Added)
	}
}
