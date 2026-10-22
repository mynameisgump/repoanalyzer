// Package gitlog streams `git log --numstat` and parses it into commits.
// Everything history-based is computed from this single pass.
package gitlog

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/mynameisgump/repoanalyzer/internal/gitx"
)

// FileChange is one line of --numstat output.
type FileChange struct {
	Path    string
	Added   int
	Deleted int
	Binary  bool
}

// Commit is a single commit with its per-file line changes.
type Commit struct {
	Hash        string
	AuthorName  string
	AuthorEmail string
	When        time.Time
	Parents     []string // first parent first; a merge has two or more
	Subject     string
	Files       []FileChange
}

const (
	recordSep = '\x1e'
	fieldSep  = "\x1f"
	// %aN/%aE honor .mailmap, so one person with several emails can be merged.
	format = "%x1e%H%x1f%aN%x1f%aE%x1f%at%x1f%P%x1f%s"
	// Same fields, but %ct: when the commit landed rather than when it was written.
	mainlineFormat = "%x1e%H%x1f%aN%x1f%aE%x1f%ct%x1f%P%x1f%s"
)

// Read runs git log in dir. since is passed to --since (e.g. "1 year ago") and may be empty.
func Read(ctx context.Context, dir, since string) ([]Commit, error) {
	return read(ctx, dir, since, format)
}

// ReadMainline reads only the current branch's first-parent chain, with each merge
// diffed against its first parent and dated by committer date. That records when
// changes actually landed on the branch, which is what line totals over time need;
// Read would place a feature branch's commits at the dates they were written.
func ReadMainline(ctx context.Context, dir, since string) ([]Commit, error) {
	return read(ctx, dir, since, mainlineFormat, "--first-parent", "--diff-merges=first-parent")
}

func read(ctx context.Context, dir, since, format string, extra ...string) ([]Commit, error) {
	if !gitx.HasCommits(ctx, dir) {
		return nil, nil
	}
	args := []string{"-c", "core.quotepath=off", "log", "--numstat", "--no-color", "--format=" + format}
	args = append(args, extra...)
	if since != "" {
		args = append(args, "--since="+since)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	commits, parseErr := Parse(out)
	if parseErr != nil {
		_ = cmd.Process.Kill()
	}
	if err := cmd.Wait(); err != nil && parseErr == nil {
		return nil, fmt.Errorf("git log: %s", strings.TrimSpace(stderr.String()))
	}
	return commits, parseErr
}

// Parse reads the output of git log using this package's format plus --numstat.
func Parse(r io.Reader) ([]Commit, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var commits []Commit
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		if line[0] == recordSep {
			c, err := parseHeader(line[1:])
			if err != nil {
				return nil, err
			}
			commits = append(commits, c)
			continue
		}
		if len(commits) == 0 {
			continue
		}
		if fc, ok := parseNumstat(line); ok {
			last := &commits[len(commits)-1]
			last.Files = append(last.Files, fc)
		}
	}
	return commits, sc.Err()
}

func parseHeader(s string) (Commit, error) {
	f := strings.SplitN(s, fieldSep, 6)
	if len(f) != 6 {
		return Commit{}, fmt.Errorf("malformed commit header: %q", s)
	}
	ts, err := strconv.ParseInt(f[3], 10, 64)
	if err != nil {
		return Commit{}, fmt.Errorf("bad timestamp in %q: %w", s, err)
	}
	return Commit{
		Hash:        f[0],
		AuthorName:  f[1],
		AuthorEmail: f[2],
		When:        time.Unix(ts, 0),
		Parents:     strings.Fields(f[4]),
		Subject:     f[5],
	}, nil
}

// parseNumstat parses "added\tdeleted\tpath"; binary files show "-" for both counts.
func parseNumstat(line string) (FileChange, bool) {
	parts := strings.SplitN(line, "\t", 3)
	if len(parts) != 3 {
		return FileChange{}, false
	}
	fc := FileChange{Path: renamedPath(parts[2])}
	if parts[0] == "-" && parts[1] == "-" {
		fc.Binary = true
		return fc, true
	}
	a, err1 := strconv.Atoi(parts[0])
	d, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return FileChange{}, false
	}
	fc.Added, fc.Deleted = a, d
	return fc, true
}

// renamedPath resolves numstat rename notation to the new path:
// "src/{old => new}/f.go" -> "src/new/f.go", "old.go => new.go" -> "new.go".
func renamedPath(p string) string {
	if i := strings.IndexByte(p, '{'); i >= 0 {
		if j := strings.IndexByte(p[i:], '}'); j >= 0 {
			j += i
			if _, after, ok := strings.Cut(p[i+1:j], " => "); ok {
				res := p[:i] + after + p[j+1:]
				res = strings.ReplaceAll(res, "//", "/")
				return strings.TrimPrefix(res, "/")
			}
		}
	}
	if _, after, ok := strings.Cut(p, " => "); ok {
		return after
	}
	return p
}
