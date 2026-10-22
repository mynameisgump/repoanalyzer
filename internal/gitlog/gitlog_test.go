package gitlog

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	in := "\x1eaaa\x1fAda\x1fada@x.io\x1f1700000000\x1fp0\x1fAdd parser (#12)\n" +
		"\n" +
		"10\t2\tmain.go\n" +
		"-\t-\tlogo.png\n" +
		"3\t3\tsrc/{old => new}/f.go\n" +
		"\x1ebbb\x1fBob\x1fbob@x.io\x1f1700000100\x1fp1 p2\x1fMerge pull request #7 from b/x\n"

	commits, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("got %d commits, want 2", len(commits))
	}
	c := commits[0]
	if c.AuthorName != "Ada" || c.Subject != "Add parser (#12)" || c.When.Unix() != 1700000000 {
		t.Errorf("bad header: %+v", c)
	}
	if len(c.Parents) != 1 || len(commits[1].Parents) != 2 || commits[1].Parents[1] != "p2" {
		t.Errorf("bad parents: %v, %v", c.Parents, commits[1].Parents)
	}
	if len(c.Files) != 3 {
		t.Fatalf("got %d files, want 3", len(c.Files))
	}
	if f := c.Files[0]; f.Path != "main.go" || f.Added != 10 || f.Deleted != 2 {
		t.Errorf("bad numstat: %+v", f)
	}
	if !c.Files[1].Binary {
		t.Errorf("logo.png should be binary")
	}
	if c.Files[2].Path != "src/new/f.go" {
		t.Errorf("rename not resolved: %q", c.Files[2].Path)
	}
	if len(commits[1].Files) != 0 {
		t.Errorf("merge commit should have no files")
	}
}

func TestRenamedPath(t *testing.T) {
	cases := map[string]string{
		"plain.go":           "plain.go",
		"old.go => new.go":   "new.go",
		"a/{b => c}/d.go":    "a/c/d.go",
		"a/{ => c}/d.go":     "a/c/d.go",
		"a/{b => }/d.go":     "a/d.go",
		"{old => new}/x.go":  "new/x.go",
		"{old => }/x.go":     "x.go",
		"weird{name}.go":     "weird{name}.go",
		"dir/{a.go => b.go}": "dir/b.go",
	}
	for in, want := range cases {
		if got := renamedPath(in); got != want {
			t.Errorf("renamedPath(%q) = %q, want %q", in, got, want)
		}
	}
}
