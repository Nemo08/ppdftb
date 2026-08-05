package convert

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCollectAndMergeXML(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// Создаём XML-файл
	xmlPath := filepath.Join(dir, "data.xml")
	if err := os.WriteFile(xmlPath, []byte(`<root><name>Test</name></root>`), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &WconvPipeline{
		DxFlags: []string{xmlPath},
	}

	merged, paths, err := collectAndMergeXML(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) == 0 {
		t.Error("expected non-empty merged data")
	}
	if len(paths) != 1 {
		t.Errorf("paths = %d, want 1", len(paths))
	}
}

func TestCollectAndMergeXMLFromDir(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// Создаём папку с XML
	xmlDir := filepath.Join(dir, "xml")
	if err := os.Mkdir(xmlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xmlDir, "data.xml"), []byte(`<root><city>Moscow</city></root>`), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &WconvPipeline{
		DxF: xmlDir,
		DxL: 1,
	}

	merged, paths, err := collectAndMergeXML(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) == 0 {
		t.Error("expected non-empty merged data from dir")
	}
	if len(paths) == 0 {
		t.Error("expected non-empty paths from dir")
	}
}

func TestCollectAndMergeXMLNoData(t *testing.T) {
	ctx := context.Background()
	p := &WconvPipeline{}

	merged, paths, err := collectAndMergeXML(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 0 {
		t.Errorf("expected empty merged data, got %d bytes", len(merged))
	}
	if len(paths) != 0 {
		t.Errorf("expected empty paths, got %d", len(paths))
	}
}

func TestCollectAndMergeXMLInvalidXml(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	badXml := filepath.Join(dir, "bad.xml")
	if err := os.WriteFile(badXml, []byte(`not xml {{{`), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &WconvPipeline{
		DxFlags: []string{badXml},
	}

	_, _, err := collectAndMergeXML(ctx, p)
	if err == nil {
		t.Error("expected error for invalid XML")
	}
}

func TestResolveTempDir(t *testing.T) {
	t.Run("explicit dir", func(t *testing.T) {
		dir := t.TempDir()
		result, err := resolveTempDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if result != dir {
			t.Errorf("resolveTempDir(%q) = %q, want %q", dir, result, dir)
		}
	})

	t.Run("empty creates temp", func(t *testing.T) {
		result, err := resolveTempDir("")
		if err != nil {
			t.Fatal(err)
		}
		if result == "" {
			t.Error("expected non-empty temp dir")
		}
		if !filepath.IsAbs(result) {
			t.Errorf("expected absolute path, got %q", result)
		}
		// Проверяем что директория существует
		info, err := os.Stat(result)
		if err != nil {
			t.Errorf("temp dir does not exist: %v", err)
		}
		if !info.IsDir() {
			t.Error("temp path is not a directory")
		}
		// Чистим
		_ = os.RemoveAll(result)
	})
}

func TestCommitCacheNoCache(t *testing.T) {
	p := &WconvPipeline{
		UseCache: false,
	}
	err := commitCache(p, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestCommitCacheNilCache(t *testing.T) {
	p := &WconvPipeline{
		UseCache: true,
		Cache:    nil,
	}
	err := commitCache(p, nil)
	if err != nil {
		t.Errorf("expected nil error for nil cache, got %v", err)
	}
}

func TestWconvPipelineEmptyRun(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	p := &WconvPipeline{
		Src:      dir,
		Out:      filepath.Join(dir, "out"),
		UseCache: false,
	}

	// Создаём пустой PDF в out (чтобы не конвертировать всё)
	if err := os.MkdirAll(p.Out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Out, "empty.pdf"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	err := RunWconvWithPool(ctx, &mockWordPool{}, p)
	if err != nil {
		t.Fatal(err)
	}
}
