package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "test.txt")
	content := []byte("hello world")

	err := WriteFileAtomic(dst, func(tmpPath string) error {
		return os.WriteFile(tmpPath, content, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestWriteFileAtomicFnError(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "test.txt")

	err := WriteFileAtomic(dst, func(tmpPath string) error {
		return os.ErrNotExist
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Error("target file should not exist after fn error")
	}
}

func TestWriteFileAtomicOverwrite(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "test.txt")

	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := WriteFileAtomic(dst, func(tmpPath string) error {
		return os.WriteFile(tmpPath, []byte("new"), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(dst)
	if string(got) != "new" {
		t.Errorf("got %q, want %q", got, "new")
	}
}

func TestWriteFileAtomicTmpFileCleaned(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "test.txt")

	err := WriteFileAtomic(dst, func(tmpPath string) error {
		if err := os.WriteFile(tmpPath, []byte("data"), 0o644); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temporary file %s was not cleaned up", e.Name())
		}
	}
}
