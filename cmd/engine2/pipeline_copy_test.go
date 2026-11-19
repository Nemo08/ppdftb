package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "dst.txt")
	if err := copyFile(dst, src); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world" {
		t.Errorf("copyFile = %q, want %q", got, "hello world")
	}
}

func TestCopyFileSrcNotFound(t *testing.T) {
	dir := t.TempDir()
	err := copyFile(filepath.Join(dir, "dst"), filepath.Join(dir, "nope"))
	if err == nil {
		t.Error("expected error for nonexistent src")
	}
}

func TestCopyFileEmptyFile(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "empty")
	if err := os.WriteFile(src, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "empty_out")
	if err := copyFile(dst, src); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("copyFile empty = %d bytes, want 0", len(got))
	}
}

func TestCopyFileLargeFile(t *testing.T) {
	dir := t.TempDir()

	// 1 MB файла
	data := make([]byte, 1<<20)
	for i := range data {
		data[i] = byte(i % 256)
	}

	src := filepath.Join(dir, "large")
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "large_out")
	if err := copyFile(dst, src); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(data) {
		t.Errorf("copyFile large = %d bytes, want %d", len(got), len(data))
	}
	if !bytes.Equal(got, data) {
		t.Error("copyFile large: content mismatch")
	}
}

func TestCopyFileOverwrite(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "dst")
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(dst, src); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("copyFile overwrite = %q, want %q", got, "new")
	}
}
