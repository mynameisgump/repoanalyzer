package loc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCount(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package a\n\nfunc A() {}\n")
	write("b.go", "package a") // no trailing newline
	write("c.py", "x = 1\n   \n")
	write("bin.dat", "ab\x00cd")
	write("go.sum", "lots\nof\nhashes\n")
	write("a_test.go", "package a\n\nfunc TestA() {}\n")

	r := Count(dir, []string{"a.go", "b.go", "c.py", "bin.dat", "go.sum", "missing.go", "a_test.go"})
	if r.Files != 4 || r.Lines != 9 || r.Blank != 3 || r.Code != 6 || r.TestFiles != 1 || r.TestCode != 2 {
		t.Errorf("totals = %+v", r)
	}
	if r.Skipped != 3 {
		t.Errorf("skipped = %d, want 3", r.Skipped)
	}
	if len(r.Languages) != 2 || r.Languages[0].Name != "Go" || r.Languages[0].Code != 5 || r.Languages[0].TestFiles != 1 {
		t.Errorf("languages = %+v", r.Languages)
	}
	if r.Largest[0].Path != "a.go" { // ties break by path
		t.Errorf("largest = %+v", r.Largest)
	}
}

func TestDetect(t *testing.T) {
	for in, want := range map[string]string{
		"main.go": "Go", "src/App.TSX": "TypeScript", "Makefile": "Makefile",
		"scenes/level.tscn": "Godot Scene", "LICENSE": "Other",
	} {
		if got := Detect(in); got != want {
			t.Errorf("Detect(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsTest(t *testing.T) {
	for in, want := range map[string]bool{
		"pkg/a_test.go":                   true,
		"src/App.test.tsx":                true,
		"src/util.spec.js":                true,
		"src/__tests__/util.js":           true,
		"tests/test_api.py":               true,
		"app/test_models.py":              true,
		"conftest.py":                     true,
		"spec/models/user_spec.rb":        true,
		"src/test/java/a/FooTest.java":    true,
		"Sources/AppTests/AppTests.swift": true,
		"MyApp.Tests/ParserTests.cs":      true,
		"internal/x/testdata/in.json":     true,
		"pkg/a.go":                        false,
		"src/Contest.java":                false,
		"src/latest.ts":                   false,
		"src/testing.go":                  false,
		"README.md":                       false,
	} {
		if got := IsTest(in); got != want {
			t.Errorf("IsTest(%q) = %v, want %v", in, got, want)
		}
	}
}
