package main

import (
	"os"
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

func TestRunNonexistentDir(t *testing.T) {
	code := run([]string{"-d", "nonexistent"})
	if code != 1 {
		t.Errorf("run(-d nonexistent) = %d, want 1", code)
	}
}

func TestRunEmptyDir(t *testing.T) {
	dir := t.TempDir()
	code := run([]string{"-d", dir})
	if code != 0 {
		t.Errorf("run(-d tempdir) = %d, want 0", code)
	}
}

func TestRunWithAppendix(t *testing.T) {
	dir := t.TempDir()
	code := run([]string{"-d", dir, "-appendix"})
	if code != 0 {
		t.Errorf("run(-d tempdir -appendix) = %d, want 0", code)
	}
}

func TestRunWithNonPdfFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "out.pdf")
	code := run([]string{"-d", dir, "-o", out})
	if code != 0 {
		t.Errorf("run(-d tempdir -o out) = %d, want 0", code)
	}
	// Без PDF файлов Merge возвращает nil и не создаёт out.pdf.
}
