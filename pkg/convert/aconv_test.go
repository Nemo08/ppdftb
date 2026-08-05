package convert

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// mockCadConverter — mock для CadConverter.
type mockCadConverter struct {
	calls []string
	mu    sync.Mutex
	err   error
}

func (m *mockCadConverter) AcadToPdf(_ context.Context, from, to string) error {
	m.mu.Lock()
	m.calls = append(m.calls, from)
	m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	// Создаём "результат" — пустой PDF
	return os.WriteFile(filepath.Join(to, "out.pdf"), []byte("%PDF-1.4"), 0o644)
}

func (m *mockCadConverter) Close() {}

// mockPdfMerger — mock для PdfMerger.
type mockPdfMerger struct {
	calls []string
	mu    sync.Mutex
	err   error
}

func (m *mockPdfMerger) Merge(_ context.Context, from, to string) error {
	m.mu.Lock()
	m.calls = append(m.calls, to)
	m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	return os.WriteFile(to, []byte("%PDF-1.4 merged"), 0o644)
}

// --- A2pdfWithPool ---

func TestA2pdfWithPool(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mockCad := &mockCadConverter{}
	mockMerger := &mockPdfMerger{}

	// Создаём фейковые DWG файлы
	files := []string{
		filepath.Join(dir, "test1.dwg"),
		filepath.Join(dir, "test2.dxf"),
	}
	for _, f := range files {
		if err := os.WriteFile(f, []byte("fake dwg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	err := A2pdfWithPool(ctx, mockCad, mockMerger, files, outDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(mockCad.calls) != 2 {
		t.Errorf("CadToPdf called %d times, want 2", len(mockCad.calls))
	}
	if len(mockMerger.calls) != 2 {
		t.Errorf("Merge called %d times, want 2", len(mockMerger.calls))
	}

	// Проверяем что выходные PDF созданы
	for _, name := range []string{"test1.pdf", "test2.pdf"} {
		path := filepath.Join(outDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("output file %s not found", name)
		}
	}
}

func TestA2pdfWithPoolEmptyFiles(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mockCad := &mockCadConverter{}
	mockMerger := &mockPdfMerger{}

	err := A2pdfWithPool(ctx, mockCad, mockMerger, nil, outDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(mockCad.calls) != 0 {
		t.Errorf("CadToPdf called %d times, want 0", len(mockCad.calls))
	}
}

func TestA2pdfWithPoolCadError(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mockCad := &mockCadConverter{
		err: os.ErrPermission,
	}
	mockMerger := &mockPdfMerger{}

	file := filepath.Join(dir, "bad.dwg")
	if err := os.WriteFile(file, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := A2pdfWithPool(ctx, mockCad, mockMerger, []string{file}, outDir)
	if err == nil {
		t.Error("expected error from CadToPdf")
	}
}

func TestA2pdfWithPoolMergeError(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mockCad := &mockCadConverter{}
	mockMerger := &mockPdfMerger{
		err: os.ErrPermission,
	}

	file := filepath.Join(dir, "test.dwg")
	if err := os.WriteFile(file, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := A2pdfWithPool(ctx, mockCad, mockMerger, []string{file}, outDir)
	if err == nil {
		t.Error("expected error from Merge")
	}
}

func TestA2pdfWithPoolMultipleFiles(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mockCad := &mockCadConverter{}
	mockMerger := &mockPdfMerger{}

	files := make([]string, 5)
	for i := range 5 {
		f := filepath.Join(dir, "test"+string(rune('0'+i))+".dwg")
		files[i] = f
		if err := os.WriteFile(f, []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	err := A2pdfWithPool(ctx, mockCad, mockMerger, files, outDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(mockCad.calls) != 5 {
		t.Errorf("CadToPdf called %d times, want 5", len(mockCad.calls))
	}
	if len(mockMerger.calls) != 5 {
		t.Errorf("Merge called %d times, want 5", len(mockMerger.calls))
	}
}

func TestA2pdfWithPoolCfg(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mockCad := &mockCadConverter{}
	mockMerger := &mockPdfMerger{}

	file := filepath.Join(dir, "test.dwg")
	if err := os.WriteFile(file, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := A2pdfWithPoolCfg(ctx, mockCad, mockMerger, []string{file}, outDir, AconvOptions{
		TempPrefix: "custom-",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(mockCad.calls) != 1 {
		t.Errorf("CadToPdf called %d times, want 1", len(mockCad.calls))
	}
}

func TestA2pdfWithPoolCfgEmptyPrefix(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mockCad := &mockCadConverter{}
	mockMerger := &mockPdfMerger{}

	file := filepath.Join(dir, "test.dwg")
	if err := os.WriteFile(file, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Empty TempPrefix должен дефолтиться к "aconv-"
	err := A2pdfWithPoolCfg(ctx, mockCad, mockMerger, []string{file}, outDir, AconvOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if len(mockCad.calls) != 1 {
		t.Errorf("CadToPdf called %d times, want 1", len(mockCad.calls))
	}
}
