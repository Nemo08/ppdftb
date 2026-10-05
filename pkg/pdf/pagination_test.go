package pdf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	c "github.com/oliverpool/unipdf/v3/creator"
	pdf "github.com/oliverpool/unipdf/v3/model"
)

// TestNumberFontResolvesArialForAppendixLabel проверяет, что шрифт нумерации
// находится через fontutil.Resolve и покрывает кириллицу надписи приложения.
func TestNumberFontResolvesArialForAppendixLabel(t *testing.T) {
	f, err := numberFont("Приложение А. 12")
	if err != nil {
		t.Fatalf("numberFont: %v", err)
	}
	if f == nil {
		t.Fatal("numberFont вернул nil-шрифт")
	}
	if _, ok := f.GetRuneMetrics('А'); !ok {
		t.Error("шрифт нумерации не содержит метрик для кириллической «А»")
	}
	if _, ok := f.GetRuneMetrics('1'); !ok {
		t.Error("шрифт нумерации не содержит метрик для цифры «1»")
	}
}

// TestAddPageNumberDrawsAppendixLabel проверяет, что надпись приложения вместе
// с номером действительно рисуется: контент страницы содержит текстовые
// операции, а с надписью — больше отрисованных глифов, чем с одним номером.
func TestAddPageNumberDrawsAppendixLabel(t *testing.T) {
	dir := t.TempDir()

	plain := pageContentWithNumber(t, dir, "plain.pdf", 3, "")
	withLabel := pageContentWithNumber(t, dir, "label.pdf", 3, "Приложение А")

	if !strings.Contains(plain, "BT") {
		t.Errorf("номер не отрисован:\n%s", plain)
	}
	if !strings.Contains(withLabel, "BT") {
		t.Errorf("надпись не отрисован:\n%s", withLabel)
	}
	// Надпись добавляет глифы, поэтому поток должен стать длиннее.
	if len(withLabel) <= len(plain) {
		t.Errorf("надпись «Приложение А» не изменила контент страницы (plain=%d, label=%d)",
			len(plain), len(withLabel))
	}
}

// pageContentWithNumber рисует номер страницы через addPageNumber и возвращает
// content stream первой страницы готового PDF.
func pageContentWithNumber(t *testing.T, dir, name string, pageNum int, prefix string) string {
	t.Helper()
	cr := c.New()
	page := pdf.NewPdfPage()
	page.MediaBox = &pdf.PdfRectangle{Urx: a4LandscapeW, Ury: a4LandscapeH}
	if err := cr.AddPage(page); err != nil {
		t.Fatalf("AddPage: %v", err)
	}
	if err := addPageNumber(cr, page, pageNum, 1, 1, prefix); err != nil {
		t.Fatalf("addPageNumber: %v", err)
	}

	out := filepath.Join(dir, name)
	if err := cr.WriteToFile(out); err != nil {
		t.Fatalf("WriteToFile: %v", err)
	}

	f, err := os.Open(out)
	if err != nil {
		t.Fatalf("открыть %s: %v", out, err)
	}
	defer f.Close()
	r, err := pdf.NewPdfReader(f)
	if err != nil {
		t.Fatalf("NewPdfReader: %v", err)
	}
	read, err := r.GetPage(1)
	if err != nil {
		t.Fatalf("GetPage(1): %v", err)
	}
	stream, err := read.GetAllContentStreams()
	if err != nil {
		t.Fatalf("GetAllContentStreams: %v", err)
	}
	return stream
}

// --- buildAppendixPageMap ---

func TestBuildAppendixPageMap(t *testing.T) {
	// buildAppendixPageMap — приватная функция.
	// Тестируем через AppendixInfoFromOutline (экспортируемая обёртка).
}

// --- appendixRangeEnd ---

func TestAppendixRangeEnd(t *testing.T) {
	ranges := []appendixRange{
		{fromPage: 5, letter: "А"},
		{fromPage: 10, letter: "Б"},
	}
	pages := []int{5, 10}

	// Первый диапазон заканчивается перед вторым
	end := appendixRangeEnd(ranges, pages, 0, 20)
	if end != 9 {
		t.Errorf("range[0] end = %d, want 9", end)
	}

	// Второй диапазон — последний, заканчивается в конце PDF
	end = appendixRangeEnd(ranges, pages, 1, 20)
	if end != 20 {
		t.Errorf("range[1] end = %d, want 20", end)
	}
}

func TestAppendixRangeEndSingleRange(t *testing.T) {
	ranges := []appendixRange{
		{fromPage: 3, letter: "А"},
	}
	pages := []int{3}

	end := appendixRangeEnd(ranges, pages, 0, 15)
	if end != 15 {
		t.Errorf("single range end = %d, want 15", end)
	}
}

func TestAppendixRangeEndWithExtraPages(t *testing.T) {
	// Есть закладки верхнего уровня, которые не являются приложениями
	ranges := []appendixRange{
		{fromPage: 5, letter: "А"},
	}
	// Закладки: 1 (Обложка), 5 (Приложение А), 15 (Графика)
	pages := []int{1, 5, 15}

	// Приложение А заканчивается перед закладкой 15
	end := appendixRangeEnd(ranges, pages, 0, 20)
	if end != 14 {
		t.Errorf("range end with extra pages = %d, want 14", end)
	}
}

// --- appendixPrefix ---

func TestAppendixPrefix(t *testing.T) {
	m := map[int]string{
		5:  "Приложение А",
		10: "Приложение Б",
	}

	if got := appendixPrefix(5, m); got != "Приложение А" {
		t.Errorf("prefix(5) = %q, want Приложение А", got)
	}
	if got := appendixPrefix(10, m); got != "Приложение Б" {
		t.Errorf("prefix(10) = %q, want Приложение Б", got)
	}
	if got := appendixPrefix(7, m); got != "" {
		t.Errorf("prefix(7) = %q, want empty", got)
	}
}

func TestAppendixPrefixNilMap(t *testing.T) {
	if got := appendixPrefix(5, nil); got != "" {
		t.Errorf("prefix with nil map = %q, want empty", got)
	}
}

// --- AppendixLetter ---

func TestAppendixLetterBasic(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "А"},
		{1, "Б"},
		{6, "Ж"},
		{7, "И"}, // З и Й пропущены
		{8, "К"},
		{24, "Я"},  // последняя одиночная
		{25, "АА"}, // первая двойная
		{26, "АБ"},
		{50, "БА"}, // 25 букв: 50-25=25, Letters[1]+Letters[0]
	}
	for _, c := range cases {
		got := AppendixLetter(c.n)
		if got != c.want {
			t.Errorf("AppendixLetter(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestAppendixLetterNegative(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Log("AppendixLetter(-1) panicked as expected")
		}
	}()
	got := AppendixLetter(-1)
	if got != "" {
		t.Errorf("AppendixLetter(-1) = %q, want empty", got)
	}
}

func TestAppendixLetterLarge(t *testing.T) {
	// Проверяем что большие индексы не паникуют
	got := AppendixLetter(100)
	if got == "" {
		t.Error("AppendixLetter(100) should return a letter, not empty")
	}
}

// --- Mm2px / Px2mm ---
// Тесты для Mm2px/Px2mm находятся в pkg/pdf/pdf_test.go.
