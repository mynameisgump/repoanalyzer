package loc

import (
	"path"
	"strings"
)

// Directories whose contents are tests or test support (fixtures, mocks, snapshots).
var testDirs = map[string]bool{
	"test": true, "tests": true, "__tests__": true, "spec": true, "specs": true,
	"e2e": true, "testdata": true, "__mocks__": true, "__snapshots__": true, "cypress": true,
}

// IsTest guesses whether a path is a test file from common naming conventions:
// a test directory anywhere in the path (tests/, __tests__/, src/test/, spec/, …,
// or a .NET "*.Tests" project), or a test file name (foo_test.go, foo.test.ts,
// foo.spec.js, test_foo.py, foo_spec.rb, FooTest.java, FooTests.swift, conftest.py).
// Tests that live inline in source files, like Rust's #[cfg(test)], aren't detected.
func IsTest(p string) bool {
	dir, base := path.Split(p)
	for _, d := range strings.Split(strings.TrimSuffix(dir, "/"), "/") {
		ld := strings.ToLower(d)
		if testDirs[ld] || strings.HasSuffix(ld, ".tests") || strings.HasSuffix(ld, ".test") {
			return true
		}
	}

	stem := strings.TrimSuffix(base, path.Ext(base))
	lower := strings.ToLower(stem)
	if lower == "conftest" || strings.HasPrefix(lower, "test_") ||
		strings.HasSuffix(lower, "_test") || strings.HasSuffix(lower, "_spec") ||
		strings.HasSuffix(lower, ".test") || strings.HasSuffix(lower, ".spec") {
		return true
	}
	// CamelCase suffixes are matched case-sensitively so Contest.java or Latest.kt don't count.
	for _, suffix := range []string{"Test", "Tests", "Spec"} {
		if s, ok := strings.CutSuffix(stem, suffix); ok && s != "" {
			return true
		}
	}
	return false
}
