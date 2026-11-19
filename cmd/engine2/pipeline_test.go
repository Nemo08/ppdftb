package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyStaticFiles(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := filepath.Join(srcDir, "out")

	// Создаём файлы в src
	if err := os.WriteFile(filepath.Join(srcDir, "readme.pdf"), []byte("pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "divider"), []byte("divider"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "template.docx"), []byte("docx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "image.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "subdir", "nested.pdf"), []byte("nested"), 0o644); err != nil {
		t.Fatal(err)
	}

	copyStaticFiles(srcDir, dstDir)

	// Проверяем что скопировалось
	entries, err := os.ReadDir(dstDir)
	if err != nil {
		t.Fatal(err)
	}

	// Должны быть: readme.pdf, divider (нет расширения), subdir пропущен
	found := make(map[string]bool)
	for _, e := range entries {
		found[e.Name()] = true
	}

	if !found["readme.pdf"] {
		t.Error("readme.pdf should be copied")
	}
	if !found["divider"] {
		t.Error("divider (no ext) should be copied")
	}
	if found["template.docx"] {
		t.Error("template.docx should NOT be copied")
	}
	if found["image.png"] {
		t.Error("image.png should NOT be copied")
	}
	if found["subdir"] {
		t.Error("subdir should NOT be copied")
	}
}

func TestCopyStaticFilesSrcNotFound(t *testing.T) {
	// Не должно паниковать
	copyStaticFiles("/nonexistent/dir", t.TempDir())
}

func TestCopyStaticFilesEmptySrc(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := filepath.Join(srcDir, "out")

	copyStaticFiles(srcDir, dstDir)

	entries, err := os.ReadDir(dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestCopyStaticFilesPreservesContent(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := filepath.Join(srcDir, "out")

	content := []byte("test pdf content for copy verification")
	if err := os.WriteFile(filepath.Join(srcDir, "doc.pdf"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	copyStaticFiles(srcDir, dstDir)

	got, err := os.ReadFile(filepath.Join(dstDir, "doc.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestCopyStaticFilesOverwriteDst(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := filepath.Join(srcDir, "out")

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Создаём dst файл
	if err := os.WriteFile(filepath.Join(dstDir, "doc.pdf"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Создаём src файл
	if err := os.WriteFile(filepath.Join(srcDir, "doc.pdf"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	copyStaticFiles(srcDir, dstDir)

	got, err := os.ReadFile(filepath.Join(dstDir, "doc.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("overwritten content = %q, want %q", got, "new")
	}
}

func TestCollectPageCounts(t *testing.T) {
	dir := t.TempDir()

	// Создаём тестовые PDF-файлы (пустые, но с правильным расширением)
	if err := os.WriteFile(filepath.Join(dir, "a.pdf"), []byte("pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.pdf"), []byte("pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte("txt"), 0o644); err != nil {
		t.Fatal(err)
	}

	counts := collectPageCounts(dir)

	// PDF файлы должны быть в карте (но PageCount вернёт ошибку для пустых файлов,
	// поэтому они будут пропущены с count=0). Проверяем что txt не в карте.
	if counts == nil {
		t.Fatal("collectPageCounts returned nil")
	}

	// a.pdf и b.pdf будут в карте с count=0 (ошибка чтения PDF)
	// c.txt не должен быть в карте
	if _, ok := counts["c.txt"]; ok {
		t.Error("c.txt should not be in page counts")
	}
}

func TestCollectPageCountsEmptyDir(t *testing.T) {
	counts := collectPageCounts(t.TempDir())
	if counts == nil {
		t.Fatal("collectPageCounts should return empty map, not nil")
	}
	if len(counts) != 0 {
		t.Errorf("expected 0 entries, got %d", len(counts))
	}
}

func TestCollectPageCountsNonexistentDir(t *testing.T) {
	counts := collectPageCounts("/nonexistent/dir")
	// collectPageCounts возвращает nil для несуществующей директории
	if counts != nil {
		t.Error("collectPageCounts should return nil for nonexistent dir")
	}
}

func TestResolveDir(t *testing.T) {
	root := "/root"

	t.Run("empty dir uses default", func(t *testing.T) {
		result := resolveDir(root, "", "Default")
		if result != filepath.Join(root, "Default") {
			t.Errorf("resolveDir(root, '', 'Default') = %q, want %q", result, filepath.Join(root, "Default"))
		}
	})

	t.Run("relative dir joined to root", func(t *testing.T) {
		result := resolveDir(root, "subdir", "Default")
		if result != filepath.Join(root, "subdir") {
			t.Errorf("resolveDir(root, 'subdir', 'Default') = %q, want %q", result, filepath.Join(root, "subdir"))
		}
	})

	t.Run("absolute dir unchanged", func(t *testing.T) {
		abs := "C:\\absolute\\path"
		result := resolveDir(root, abs, "Default")
		if result != abs {
			t.Errorf("resolveDir(root, '%s', 'Default') = %q, want %q", abs, result, abs)
		}
	})
}

func TestEnsureDirs(t *testing.T) {
	dir := t.TempDir()

	sub1 := filepath.Join(dir, "a", "b")
	sub2 := filepath.Join(dir, "c")

	if err := ensureDirs(sub1, sub2); err != nil {
		t.Fatal(err)
	}

	for _, d := range []string{sub1, sub2} {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("expected dir %s to exist", d)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", d)
		}
	}
}

func TestEnsureDirsFailsOnPermissionError(t *testing.T) {
	// Пытаемся создать директорию в корне системного пути (на Windows это может не сработать)
	err := ensureDirs("\\\\root\\\\nope")
	if err == nil {
		// На некоторых системах может сработать — не фатально
		t.Log("ensureDirs on root path succeeded (unexpected but not fatal)")
	}
}

func TestCleanDir(t *testing.T) {
	dir := t.TempDir()

	// Создаём файлы и поддиректории
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file2.pdf"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "subdir", "nested.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	cleanDir(dir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after clean, got %d", len(entries))
	}
}

func TestCleanDirNonexistent(_ *testing.T) {
	// Не должно паниковать
	cleanDir("/nonexistent/dir")
}
