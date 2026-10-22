// Package loc counts lines in the files git tracks, grouped by language.
package loc

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/mynameisgump/repoanalyzer/internal/gitx"
)

// Files larger than this are assumed to be data or generated and are skipped.
const maxFileSize = 4 << 20

// How many of the largest files Result.Largest keeps.
const largestLimit = 50

// Lockfiles are tracked but are not code anyone wrote, so they'd skew the totals.
var ignoredNames = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true, "bun.lockb": true,
	"go.sum": true, "Cargo.lock": true, "poetry.lock": true, "Pipfile.lock": true,
	"composer.lock": true, "Gemfile.lock": true, "uv.lock": true,
}

type Language struct {
	Name  string `json:"name"`
	Files int    `json:"files"`
	Lines int    `json:"lines"`
	Blank int    `json:"blank"`
	Code  int    `json:"code"`
	// The part of Files and Code that is tests; see IsTest.
	TestFiles int `json:"test_files"`
	TestCode  int `json:"test_code"`
}

type File struct {
	Path     string `json:"path"`
	Language string `json:"language"`
	Lines    int    `json:"lines"`
}

type Result struct {
	Files     int        `json:"files"`
	Lines     int        `json:"lines"`
	Blank     int        `json:"blank"`
	Code      int        `json:"code"`
	TestFiles int        `json:"test_files"` // the part of Files and Code that is tests; see IsTest
	TestCode  int        `json:"test_code"`
	Skipped   int        `json:"skipped"` // binary, oversized, lockfiles, missing
	Languages []Language `json:"languages"`
	Largest   []File     `json:"largest"`
}

// IsIgnored reports whether a path is skipped as non-authored (lockfiles).
func IsIgnored(path string) bool {
	return ignoredNames[filepath.Base(path)]
}

// TrackedFiles lists paths (relative to root) that git tracks.
func TrackedFiles(ctx context.Context, root string) ([]string, error) {
	out, err := gitx.Git(ctx, root, "ls-files", "-z")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(strings.TrimRight(out, "\x00"), "\x00"), nil
}

// Count reads every path under root in parallel and tallies lines per language.
func Count(root string, paths []string) Result {
	jobs := make(chan string)
	results := make(chan *File)
	var wg sync.WaitGroup

	type langAcc struct{ files, lines, blank, testFiles, testCode int }
	var mu sync.Mutex
	langs := map[string]*langAcc{}
	skipped := 0

	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				var lines, blank int
				ok := !IsIgnored(p)
				if ok {
					lines, blank, ok = countFile(filepath.Join(root, p))
				}
				if !ok {
					mu.Lock()
					skipped++
					mu.Unlock()
					continue
				}
				lang := Detect(p)
				mu.Lock()
				a := langs[lang]
				if a == nil {
					a = &langAcc{}
					langs[lang] = a
				}
				a.files++
				a.lines += lines
				a.blank += blank
				if IsTest(p) {
					a.testFiles++
					a.testCode += lines - blank
				}
				mu.Unlock()
				results <- &File{Path: p, Language: lang, Lines: lines}
			}
		}()
	}
	go func() {
		for _, p := range paths {
			jobs <- p
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	var files []File
	for f := range results {
		files = append(files, *f)
	}

	var r Result
	for name, a := range langs {
		code := a.lines - a.blank
		r.Languages = append(r.Languages, Language{Name: name, Files: a.files, Lines: a.lines, Blank: a.blank, Code: code,
			TestFiles: a.testFiles, TestCode: a.testCode})
		r.Files += a.files
		r.TestFiles += a.testFiles
		r.TestCode += a.testCode
		r.Lines += a.lines
		r.Blank += a.blank
		r.Code += code
	}
	r.Skipped = skipped
	sort.Slice(r.Languages, func(i, j int) bool {
		if r.Languages[i].Code != r.Languages[j].Code {
			return r.Languages[i].Code > r.Languages[j].Code
		}
		return r.Languages[i].Name < r.Languages[j].Name
	})
	sort.Slice(files, func(i, j int) bool {
		if files[i].Lines != files[j].Lines {
			return files[i].Lines > files[j].Lines
		}
		return files[i].Path < files[j].Path
	})
	if len(files) > largestLimit {
		files = files[:largestLimit]
	}
	r.Largest = files
	return r
}

// countFile returns total and blank lines; ok is false for files that should not count.
func countFile(path string) (lines, blank int, ok bool) {
	info, err := os.Lstat(path)
	// Lstat so symlinks and submodule directories are skipped rather than followed.
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxFileSize {
		return 0, 0, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, false
	}
	if bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
		return 0, 0, false // binary
	}
	for len(data) > 0 {
		var line []byte
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			line, data = data[:i], data[i+1:]
		} else {
			line, data = data, nil
		}
		lines++
		if len(bytes.TrimSpace(line)) == 0 {
			blank++
		}
	}
	return lines, blank, true
}
