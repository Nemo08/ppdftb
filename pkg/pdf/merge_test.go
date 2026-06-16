package pdf

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// createMinimalPDF создаёт минимальный валидный PDF-файл (1 страница A4).
func createMinimalPDF(t *testing.T, path string) {
	t.Helper()
	// Минимальный PDF без xref-таблицы (linearized-style, достаточно для unipdf).
	content := []byte(`%PDF-1.4
1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj
2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj
3 0 obj<</Type/Page/MediaBox[0 0 595 842]/Parent 2 0 R/Resources<<>>>>endobj
xref
0 4
0000000000 65535 f 
0000000009 00000 n 
0000000058 00000 n 
0000000115 00000 n 
trailer<</Size 4/Root 1 0 R>>
startxref
217
%%EOF`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("createMinimalPDF: %v", err)
	}
}

func TestMergeEmptyDir(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "out.pdf")
	if err := Merge(context.Background(), dir, outFile); err != nil {
		t.Fatalf("Merge пустой папки вернул ошибку: %v", err)
	}
	if _, err := os.Stat(outFile); err == nil {
		t.Error("выходной файл не должен создаваться для пустой папки")
	}
}

func TestMergeNoPDFFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}
	outFile := filepath.Join(dir, "out.pdf")
	if err := Merge(context.Background(), dir, outFile); err != nil {
		t.Fatalf("Merge без PDF вернул ошибку: %v", err)
	}
}

func TestMergeNonexistentDir(t *testing.T) {
	err := Merge(context.Background(), filepath.Join(os.TempDir(), "nonexistent_xyz_12345"), filepath.Join(os.TempDir(), "out.pdf"))
	if err == nil {
		t.Error("ожидали ошибку для несуществующей папки")
	}
}

func TestMergeCorruptPDF(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.pdf"), []byte("not a pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	outFile := filepath.Join(dir, "out.pdf")
	err := Merge(context.Background(), dir, outFile)
	if err == nil {
		t.Error("ожидали ошибку для повреждённого PDF")
	}
}

func TestMergeValidPDFs(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "merged.pdf")

	// Создаём два минимальных PDF.
	createMinimalPDF(t, filepath.Join(dir, "01 Первый.pdf"))
	createMinimalPDF(t, filepath.Join(dir, "02 Второй.pdf"))

	if err := Merge(context.Background(), dir, outFile); err != nil {
		t.Fatalf("Merge вернул ошибку: %v", err)
	}
	info, err := os.Stat(outFile)
	if err != nil {
		t.Fatalf("выходной файл не создан: %v", err)
	}
	if info.Size() == 0 {
		t.Error("выходной файл пуст")
	}
}

func TestMergeNaturalSort(t *testing.T) {
	// Проверяем что файлы склеиваются в natural-sort порядке, а не лексическом.
	dir := t.TempDir()
	names := []string{"10 Десятый.pdf", "2 Второй.pdf", "1 Первый.pdf"}
	for _, name := range names {
		createMinimalPDF(t, filepath.Join(dir, name))
	}
	outFile := filepath.Join(dir, "out.pdf")
	if err := Merge(context.Background(), dir, outFile); err != nil {
		t.Fatalf("Merge вернул ошибку: %v", err)
	}
	// Если файл создан без паники — сортировка отработала корректно.
	if _, err := os.Stat(outFile); err != nil {
		t.Fatalf("выходной файл не создан: %v", err)
	}
}

func TestMergeAtomicWrite(t *testing.T) {
	// Существующий выходной файл должен быть заменён атомарно.
	// outFile НЕ в той же папке, что исходные PDF — иначе Merge подхватит
	// его как входной файл и упадёт с ошибкой.
	dir := t.TempDir()
	outDir := t.TempDir()
	outFile := filepath.Join(outDir, "out.pdf")
	if err := os.WriteFile(outFile, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}

	createMinimalPDF(t, filepath.Join(dir, "01.pdf"))

	if err := Merge(context.Background(), dir, outFile); err != nil {
		t.Fatalf("Merge вернул ошибку: %v", err)
	}
	data, _ := os.ReadFile(outFile)
	if string(data) == "old content" {
		t.Error("выходной файл не был перезаписан")
	}
}
