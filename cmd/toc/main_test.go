package main

import (
	"path/filepath"
	"testing"
)

func TestRunNoArgs(t *testing.T) {
	code := run([]string{})
	if code != 1 {
		t.Errorf("run([]) = %d, want 1", code)
	}
}

func TestRunVersion(t *testing.T) {
	code := run([]string{"-v"})
	if code != 0 {
		t.Errorf("run(-v) = %d, want 0", code)
	}
}

func TestRunMissingFlags(t *testing.T) {
	code := run([]string{"-tf", "template.docx"})
	if code != 1 {
		t.Errorf("run(-tf only) = %d, want 1", code)
	}
}

func TestRunMissingPositionalArgs(t *testing.T) {
	code := run([]string{"one", "two"})
	if code != 1 {
		t.Errorf("run(one two) = %d, want 1", code)
	}
}

func TestRunBadPageNumber(t *testing.T) {
	dir := t.TempDir()
	code := run([]string{filepath.Join(dir, "src.docx"), dir, dir, "abc"})
	if code != 1 {
		t.Errorf("run(bad page) = %d, want 1", code)
	}
}

func TestRunNonexistentTemplate(t *testing.T) {
	dir := t.TempDir()
	code := run([]string{"-tf", filepath.Join(dir, "nope.docx"), "-td", dir, "-pd", dir})
	if code != 1 {
		t.Errorf("run(-tf nonexistent) = %d, want 1", code)
	}
}

func TestRunPositionalEmptyDirs(t *testing.T) {
	dir := t.TempDir()
	code := run([]string{dir, dir, dir})
	if code != 1 {
		t.Errorf("run(3x dir) = %d, want 1 (no template/docx)", code)
	}
}

func TestRunPositionalWithAppendix(t *testing.T) {
	dir := t.TempDir()
	code := run([]string{dir, dir, dir, "-appendix"})
	if code != 1 {
		t.Errorf("run(3x dir + -appendix) = %d, want 1 (no files)", code)
	}
}
