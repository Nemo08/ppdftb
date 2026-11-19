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

func TestRunMissingOutput(t *testing.T) {
	code := run([]string{"-if", "input.pdf"})
	if code != 1 {
		t.Errorf("run(-if only) = %d, want 1", code)
	}
}

func TestRunMissingInput(t *testing.T) {
	code := run([]string{"-of", "out.pdf"})
	if code != 1 {
		t.Errorf("run(-of only) = %d, want 1", code)
	}
}

func TestRunNonexistentInput(t *testing.T) {
	dir := t.TempDir()
	code := run([]string{"-if", filepath.Join(dir, "nope.pdf"), "-of", filepath.Join(dir, "out.pdf")})
	if code != 1 {
		t.Errorf("run(nonexistent input) = %d, want 1", code)
	}
}

func TestRunWithAppendix(t *testing.T) {
	dir := t.TempDir()
	code := run([]string{"-if", filepath.Join(dir, "in.pdf"), "-of", filepath.Join(dir, "out.pdf"), "-appendix"})
	if code != 1 {
		t.Errorf("run(-appendix nonexistent) = %d, want 1", code)
	}
}
