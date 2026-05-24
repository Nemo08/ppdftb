//go:build windows

package convert

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectCadFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "drawing.dwg"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "exchange.dxf"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "document.docx"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte{}, 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "nested.dwg"), []byte{}, 0o644)

	t.Run("collect from dir (non-recursive)", func(t *testing.T) {
		files, err := CollectCadFiles("", dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 2 {
			t.Errorf("CollectCadFiles() = %d files, want 2 (drawing.dwg, exchange.dxf)", len(files))
		}
	})

	t.Run("single file", func(t *testing.T) {
		files, err := CollectCadFiles(filepath.Join(dir, "drawing.dwg"), "")
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 1 {
			t.Errorf("CollectCadFiles() = %d files, want 1", len(files))
		}
	})

	t.Run("both source and dir merge results", func(t *testing.T) {
		files, err := CollectCadFiles(filepath.Join(dir, "drawing.dwg"), dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 3 {
			t.Errorf("CollectCadFiles() = %d files, want 3 (drawing.dwg + 2 from dir)", len(files))
		}
	})

	t.Run("no cad files in dir", func(t *testing.T) {
		emptyDir := t.TempDir()
		os.WriteFile(filepath.Join(emptyDir, "readme.txt"), []byte{}, 0o644)
		files, err := CollectCadFiles("", emptyDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 0 {
			t.Errorf("CollectCadFiles() = %d files, want 0", len(files))
		}
	})

	t.Run("non-existent source", func(t *testing.T) {
		files, err := CollectCadFiles(filepath.Join(dir, "nonexistent.dwg"), "")
		if err == nil {
			t.Error("expected error for non-existent source, got nil")
		}
		if files != nil {
			t.Errorf("expected nil files, got %v", files)
		}
	})

	t.Run("both empty", func(t *testing.T) {
		files, err := CollectCadFiles("", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 0 {
			t.Errorf("CollectCadFiles() = %d files, want 0", len(files))
		}
	})
}
