//go:build windows

package convert

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTplFuncs(t *testing.T) {
	tfm := tplFuncs()

	if tfm == nil {
		t.Fatal("tplFuncs() returned nil")
	}

	t.Run("add", func(t *testing.T) {
		fn, ok := tfm["add"].(func(int, int) int)
		if !ok {
			t.Fatal("add is not func(int,int) int")
		}
		if got := fn(2, 3); got != 5 {
			t.Errorf("add(2,3) = %d, want 5", got)
		}
	})

	t.Run("year returns non-empty", func(t *testing.T) {
		fn, ok := tfm["year"].(func() string)
		if !ok {
			t.Fatal("year is not func() string")
		}
		if got := fn(); got == "" {
			t.Error("year() returned empty string")
		}
	})

	t.Run("nowdate returns date format", func(t *testing.T) {
		fn, ok := tfm["nowdate"].(func() string)
		if !ok {
			t.Fatal("nowdate is not func() string")
		}
		if got := fn(); got == "" {
			t.Error("nowdate() returned empty string")
		}
	})

	t.Run("datetime returns datetime format", func(t *testing.T) {
		fn, ok := tfm["datetime"].(func() string)
		if !ok {
			t.Fatal("datetime is not func() string")
		}
		if got := fn(); got == "" {
			t.Error("datetime() returned empty string")
		}
	})
}

func TestFilecopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	content := "hello world"

	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	n, err := filecopy(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(content)) {
		t.Errorf("filecopy() = %d bytes, want %d", n, len(content))
	}

	dstData, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(dstData) != content {
		t.Errorf("filecopy() content = %q, want %q", string(dstData), content)
	}
}

func TestFilecopyNonexistent(t *testing.T) {
	dir := t.TempDir()
	_, err := filecopy(filepath.Join(dir, "nonexistent.txt"), filepath.Join(dir, "out.txt"))
	if err == nil {
		t.Error("filecopy() expected error for nonexistent source")
	}
}

func TestCollectWordFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.docx"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "b.doc"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "c.rtf"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "d.pdf"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "~$temp.docx"), []byte{}, 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "e.docx"), []byte{}, 0o644)

	t.Run("collect from dir", func(t *testing.T) {
		files, err := CollectWordFiles([]string{dir})
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 3 {
			t.Errorf("CollectWordFiles() = %d files, want 3 (a.docx, b.doc, c.rtf — sub не рекурсивный)", len(files))
		}
	})

	t.Run("non-existent source", func(t *testing.T) {
		_, err := CollectWordFiles([]string{filepath.Join(dir, "nope")})
		if err == nil {
			t.Error("CollectWordFiles() expected error for non-existent source")
		}
	})
}
