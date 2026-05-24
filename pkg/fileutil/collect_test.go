package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasExt(t *testing.T) {
	tests := []struct {
		name string
		ext  string
		exts []string
		want bool
	}{
		{"found", ".pdf", []string{".pdf", ".docx"}, true},
		{"not found", ".txt", []string{".pdf", ".docx"}, false},
		{"nil exts", ".pdf", nil, false},
		{"empty exts", ".pdf", []string{}, false},
		{"empty ext", "", []string{".pdf"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasExt(tt.ext, tt.exts); got != tt.want {
				t.Errorf("hasExt(%q, %v) = %v, want %v", tt.ext, tt.exts, got, tt.want)
			}
		})
	}
}

func TestHasAnyPrefix(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		prefixes []string
		want     bool
	}{
		{"found", "~$test.docx", []string{"~$"}, true},
		{"not found", "test.docx", []string{"~$"}, false},
		{"nil prefixes", "test.docx", nil, false},
		{"empty prefixes", "test.docx", []string{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasAnyPrefix(tt.s, tt.prefixes); got != tt.want {
				t.Errorf("hasAnyPrefix(%q, %v) = %v, want %v", tt.s, tt.prefixes, got, tt.want)
			}
		})
	}
}

func TestCollectFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "doc.docx"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "sheet.xlsx"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "~$temp.docx"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte{}, 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "nested.docx"), []byte{}, 0o644)

	t.Run("filter by .docx ext", func(t *testing.T) {
		files, err := CollectFiles([]string{dir}, []string{".docx"})
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 2 {
			t.Errorf("CollectFiles() = %d files, want 2 (doc.docx, ~$temp.docx)", len(files))
		}
	})

	t.Run("filter by .xlsx ext", func(t *testing.T) {
		files, err := CollectFiles([]string{dir}, []string{".xlsx"})
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 1 {
			t.Errorf("CollectFiles() = %d files, want 1 (sheet.xlsx)", len(files))
		}
	})

	t.Run("with skip prefix", func(t *testing.T) {
		files, err := CollectFiles([]string{dir}, []string{".docx", ".xlsx", ".txt"}, "~$")
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 3 {
			t.Errorf("CollectFiles() with skip prefix = %d files, want 3 (doc.docx, sheet.xlsx, readme.txt)", len(files))
		}
	})

	t.Run("single file source", func(t *testing.T) {
		files, err := CollectFiles([]string{filepath.Join(dir, "doc.docx")}, []string{".docx"})
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 1 {
			t.Errorf("CollectFiles() = %d files, want 1", len(files))
		}
	})

	t.Run("non-existent source", func(t *testing.T) {
		_, err := CollectFiles([]string{filepath.Join(dir, "nope")}, []string{".docx"})
		if err == nil {
			t.Error("expected error for non-existent source")
		}
	})

	t.Run("empty sources", func(t *testing.T) {
		files, err := CollectFiles([]string{}, []string{".docx"})
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 0 {
			t.Errorf("CollectFiles() = %d files, want 0", len(files))
		}
	})
}

func TestReadFiles(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.txt")
	f2 := filepath.Join(dir, "b.txt")
	os.WriteFile(f1, []byte("hello"), 0o644)
	os.WriteFile(f2, []byte("world"), 0o644)

	t.Run("all files read", func(t *testing.T) {
		data, err := readFiles([]string{f1, f2})
		if err != nil {
			t.Fatal(err)
		}
		if len(data) != 2 {
			t.Fatalf("got %d files, want 2", len(data))
		}
		if string(data[0]) != "hello" || string(data[1]) != "world" {
			t.Errorf("unexpected content: %q, %q", string(data[0]), string(data[1]))
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := readFiles([]string{filepath.Join(dir, "nope.txt")})
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("empty input", func(t *testing.T) {
		data, err := readFiles(nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) != 0 {
			t.Errorf("got %d files, want 0", len(data))
		}
	})
}

func TestWalkUpDirs(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("walks up correct number of steps", func(t *testing.T) {
		dirs, err := walkUpDirs(sub, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(dirs) < 2 {
			t.Fatalf("got %d dirs, want at least 2", len(dirs))
		}
		if dirs[len(dirs)-1] != sub {
			t.Errorf("last dir = %q, want %q", dirs[len(dirs)-1], sub)
		}
	})

	t.Run("stops at filesystem root", func(t *testing.T) {
		dirs, err := walkUpDirs(dir, 999)
		if err != nil {
			t.Fatal(err)
		}
		if len(dirs) < 1 {
			t.Error("expected at least one dir (the start)")
		}
	})
}

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
