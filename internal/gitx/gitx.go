// Package gitx holds small helpers for running git and other CLIs.
package gitx

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Run executes name with args in dir and returns trimmed stdout.
// On failure the error includes stderr, which is usually the useful part.
func Run(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Git is shorthand for Run(ctx, dir, "git", args...).
func Git(ctx context.Context, dir string, args ...string) (string, error) {
	return Run(ctx, dir, "git", args...)
}

// RepoRoot returns the top-level directory of the work tree containing dir.
func RepoRoot(ctx context.Context, dir string) (string, error) {
	return Git(ctx, dir, "rev-parse", "--show-toplevel")
}

// HasCommits reports whether HEAD points at a commit (false in a fresh repo).
func HasCommits(ctx context.Context, dir string) bool {
	_, err := Git(ctx, dir, "rev-parse", "-q", "--verify", "HEAD")
	return err == nil
}
