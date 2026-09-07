package pdfpl

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	model "github.com/oliverpool/unipdf/v3/model"
)

// testDataDir возвращает папку tests4 (не входит в git) или пустую строку.
func testDataDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Clean(filepath.Join("..", "..", "tests4"))
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("папка tests4 не найдена (%s): %v", dir, err)
	}
	return dir
}

// testValuesFromXML собирает значения из всех tests4/*.xml (первый выигрывает).
func testValuesFromXML(t *testing.T, dir string) map[string]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.xml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("XML в %s не найдены: %v", dir, err)
	}
	out := make(map[string]string)
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			t.Fatalf("открыть %s: %v", f, err)
		}
		v, err := ValuesFromXML(fh)
		_ = fh.Close()
		if err != nil {
			t.Fatalf("ValuesFromXML(%s): %v", f, err)
		}
		for k, val := range v {
			if _, ok := out[k]; !ok {
				out[k] = val
			}
		}
	}
	return out
}

// TestFileReplaceRealPDF прогоняет ReplaceFile на реальном PDF из tests4
// (обложка тома от MS Word LTSC) и проверяет, что файл перезаписывается
// валидным PDF с заменой плейсхолдеров.
func TestFileReplaceRealPDF(t *testing.T) {
	dir := testDataDir(t)

	src := filepath.Join(dir, "Шаблон тома", "2. Обложка и титульный лист.pdf")
	dst := filepath.Join(dir, "обложка-pdfpl-out.pdf")
	t.Cleanup(func() { _ = os.Remove(dst) })

	values := testValuesFromXML(t, dir)

	res, err := ReplaceFile(context.Background(), src, dst, values)
	if err != nil {
		t.Fatalf("ReplaceFile: %v", err)
	}
	t.Logf("Pages=%d Streams=%d Replaced=%d Skipped=%d",
		res.Pages, res.Streams, res.Replaced, res.Skipped)

	if res.Pages == 0 {
		t.Fatal("Pages = 0, ожидались страницы")
	}
	if res.Replaced == 0 {
		t.Fatal("Replaced = 0, ожидалась замена плейсхолдеров")
	}
	if res.Streams == 0 {
		t.Fatal("Streams = 0, ожидалось изменение контент-потоков")
	}

	st, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("выходной файл не создан: %v", err)
	}
	if st.Size() == 0 {
		t.Fatal("выходной файл пустой")
	}

	// В выходном PDF не должно остаться видимых плейсхолдеров {{.Key}}.
	for _, keys := range perPagePlaceholders(t, dst) {
		if len(keys) != 0 {
			t.Errorf("в выходном PDF остались плейсхолдеры: %v", keys)
		}
	}
}

// perPagePlaceholders возвращает плейсхолдеры по страницам из декодированных потоков.
func perPagePlaceholders(t *testing.T, path string) map[int][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("открыть %s: %v", path, err)
	}
	defer f.Close()
	r, err := model.NewPdfReader(f)
	if err != nil {
		t.Fatalf("NewPdfReader(%s): %v", path, err)
	}
	n, err := r.GetNumPages()
	if err != nil {
		t.Fatalf("GetNumPages(%s): %v", path, err)
	}
	out := make(map[int][]string, n)
	for p := 1; p <= n; p++ {
		page, err := r.GetPage(p)
		if err != nil {
			t.Fatalf("GetPage(%d): %v", p, err)
		}
		streams, err := page.GetContentStreams()
		if err != nil {
			t.Fatalf("GetContentStreams(page %d): %v", p, err)
		}
		for _, s := range streams {
			keys, err := ListPlaceholders(s)
			if err != nil {
				t.Fatalf("ListPlaceholders(page %d): %v", p, err)
			}
			out[p] = append(out[p], keys...)
		}
	}
	return out
}

// TestFileReplaceNoChange обновляет: если значений нет, файл перезаписывается без изменений.
func TestFileReplaceNoChange(t *testing.T) {
	dir := testDataDir(t)

	src := filepath.Join(dir, "Шаблон тома", "2. Обложка и титульный лист.pdf")
	dst := filepath.Join(dir, "обложка-pdfpl-noval-out.pdf")
	t.Cleanup(func() { _ = os.Remove(dst) })

	res, err := ReplaceFile(context.Background(), src, dst, nil)
	if err != nil {
		t.Fatalf("ReplaceFile: %v", err)
	}
	if res.Streams != 0 || res.Replaced != 0 {
		t.Errorf("с пустыми values не должно быть замен: %+v", res)
	}

	_, err = os.Stat(dst)
	if err != nil {
		t.Fatalf("выходной файл не создан: %v", err)
	}
}
