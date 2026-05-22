package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsChanged(t *testing.T) {
	ref := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		cached   FileEntry
		current  FileEntry
		withHash bool
		want     bool
	}{
		{
			name:     "identical modtime and size",
			cached:   FileEntry{ModTime: ref, Size: 100},
			current:  FileEntry{ModTime: ref, Size: 100},
			withHash: false,
			want:     false,
		},
		{
			name:     "different modtime",
			cached:   FileEntry{ModTime: ref, Size: 100},
			current:  FileEntry{ModTime: ref.Add(time.Hour), Size: 100},
			withHash: false,
			want:     true,
		},
		{
			name:     "different size",
			cached:   FileEntry{ModTime: ref, Size: 100},
			current:  FileEntry{ModTime: ref, Size: 200},
			withHash: false,
			want:     true,
		},
		{
			name:     "identical hash",
			cached:   FileEntry{Hash: "abc123"},
			current:  FileEntry{Hash: "abc123"},
			withHash: true,
			want:     false,
		},
		{
			name:     "different hash",
			cached:   FileEntry{Hash: "abc123"},
			current:  FileEntry{Hash: "def456"},
			withHash: true,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isChanged(tt.cached, tt.current, tt.withHash); got != tt.want {
				t.Errorf("isChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMakeEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	entry, err := makeEntry(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Size != 5 {
		t.Errorf("Size = %d, want 5", entry.Size)
	}
	if entry.Hash != "" {
		t.Errorf("Hash should be empty when withHash=false, got %q", entry.Hash)
	}

	entryHash, err := makeEntry(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if entryHash.Hash == "" {
		t.Error("Hash should not be empty when withHash=true")
	}
}

func TestIsDirEmpty(t *testing.T) {
	t.Run("non-existent dir", func(t *testing.T) {
		got, err := isDirEmpty(filepath.Join(t.TempDir(), "nonexistent"))
		if err != nil {
			t.Fatal(err)
		}
		if !got {
			t.Error("isDirEmpty() = false, want true for non-existent dir")
		}
	})

	t.Run("empty dir", func(t *testing.T) {
		got, err := isDirEmpty(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if !got {
			t.Error("isDirEmpty() = false, want true for empty dir")
		}
	})

	t.Run("dir with PDF", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "doc.pdf"), []byte("pdf"), 0o644)
		got, err := isDirEmpty(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got {
			t.Error("isDirEmpty() = true, want false for dir with PDF")
		}
	})

	t.Run("dir with non-PDF ignored", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("text"), 0o644)
		got, err := isDirEmpty(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !got {
			t.Error("isDirEmpty() = false, want true for dir with only non-PDF")
		}
	})
}

func TestLoadSaveCache(t *testing.T) {
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	c := Cache{
		"file1.txt": FileEntry{Size: 10, ModTime: time.Now()},
		"sub/file2.txt": FileEntry{Size: 20, ModTime: time.Now()},
	}

	if err := SaveCache(c); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadCache()
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded) != 2 {
		t.Errorf("LoadCache() returned %d entries, want 2", len(loaded))
	}

	e1, ok := loaded["file1.txt"]
	if !ok {
		t.Fatal("missing file1.txt in loaded cache")
	}
	if e1.Size != 10 {
		t.Errorf("file1.txt Size = %d, want 10", e1.Size)
	}
}

func TestCollectFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.docx"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "b.doc"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "c.pdf"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "d.txt"), []byte{}, 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "e.docx"), []byte{}, 0o644)

	t.Run("filter by doc extensions", func(t *testing.T) {
		files, err := collectFiles([]string{dir}, docExts)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 3 {
			t.Errorf("collectFiles() = %d files, want 3 (a.docx, b.doc, sub/e.docx)", len(files))
		}
	})

	t.Run("empty exts = all files", func(t *testing.T) {
		files, err := collectFiles([]string{dir}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 5 {
			t.Errorf("collectFiles() with nil exts = %d files, want 5 (a.docx, b.doc, c.pdf, d.txt, sub/e.docx)", len(files))
		}
	})

	t.Run("non-existent dir", func(t *testing.T) {
		_, err := collectFiles([]string{filepath.Join(dir, "nope")}, nil)
		if err == nil {
			t.Error("collectFiles() expected error for non-existent dir")
		}
	})
}

func TestPruneCache(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "exists.txt")
	os.WriteFile(existing, []byte("data"), 0o644)

	c := Cache{
		toRel(existing):                   FileEntry{Size: 4},
		toRel(filepath.Join(dir, "gone.txt")): FileEntry{Size: 100},
	}

	pruneCache(c)

	if _, ok := c[toRel(existing)]; !ok {
		t.Error("pruneCache() removed existing file entry")
	}
	if len(c) != 1 {
		t.Errorf("pruneCache() left %d entries, want 1", len(c))
	}
}
