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
		"file1.txt":     FileEntry{Size: 10, ModTime: time.Now()},
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
		toRel(existing):                       FileEntry{Size: 4},
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

func TestToRel(t *testing.T) {
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)

	dir := t.TempDir()
	os.Chdir(dir)

	tests := []struct {
		name string
		path string
		want string
	}{
		{"absolute under wd", filepath.Join(dir, "sub", "file.txt"), filepath.Join("sub", "file.txt")},
		{"already relative", "relative/path.txt", "relative/path.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toRel(tt.path)
			if got != tt.want {
				t.Errorf("toRel(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")
	content := []byte("hello world, this is a test file for hashing")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	hash, err := hashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(hash) != 64 {
		t.Errorf("hashFile() returned %d chars, want 64 (SHA-256 hex)", len(hash))
	}

	t.Run("same content same hash", func(t *testing.T) {
		path2 := filepath.Join(dir, "copy.txt")
		os.WriteFile(path2, content, 0o644)
		hash2, _ := hashFile(path2)
		if hash != hash2 {
			t.Errorf("same content produced different hashes: %q vs %q", hash, hash2)
		}
	})

	t.Run("different content different hash", func(t *testing.T) {
		path3 := filepath.Join(dir, "other.txt")
		os.WriteFile(path3, []byte("different"), 0o644)
		hash3, _ := hashFile(path3)
		if hash == hash3 {
			t.Error("different content produced same hash")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := hashFile(filepath.Join(dir, "nonexistent"))
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}
