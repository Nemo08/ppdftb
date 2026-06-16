package pdf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendixLetter(t *testing.T) {
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
	}
	for _, c := range cases {
		got := AppendixLetter(c.n)
		if got != c.want {
			t.Errorf("AppendixLetter(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestCleanFileName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"5. Текстовая часть", "Текстовая часть"},
		{"10. ПРИЛОЖЕНИЯ", "ПРИЛОЖЕНИЯ"},
		{"60. ГРАФИЧЕСКАЯ ЧАСТЬ", "ГРАФИЧЕСКАЯ ЧАСТЬ"},
		{"1. Обложка", "Обложка"},
		{"без номера", "без номера"},
	}
	for _, c := range cases {
		got := cleanFileName(c.in)
		if got != c.want {
			t.Errorf("cleanFileName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCollectEntriesNoAppendix(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1. Обложка.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "3. Содержание.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "10. ПРИЛОЖЕНИЯ"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "11. Приложение А.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := CollectEntries(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	// Без appendix заглушка не включается
	for _, e := range entries {
		if e.Kind == KindDivider {
			t.Error("без флага appendix заглушки не должны включаться")
		}
	}
	// PDF файлы должны быть все
	var pdfCount int
	for _, e := range entries {
		if e.FullPath != "" {
			pdfCount++
		}
	}
	if pdfCount != 3 {
		t.Errorf("ожидали 3 PDF, получили %d", pdfCount)
	}
}

func TestCollectEntriesWithAppendix(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1. Обложка.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "3. Содержание.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "5. Текст.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "10. ПРИЛОЖЕНИЯ"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "11. Первое.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "12. Второе.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "60. ГРАФИЧЕСКАЯ ЧАСТЬ"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "61. Чертёж.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := CollectEntries(dir, true)
	if err != nil {
		t.Fatal(err)
	}

	byName := make(map[string]FileEntry)
	for _, e := range entries {
		byName[e.RawName] = e
	}

	// Заглушки — KindDivider
	if byName["10. ПРИЛОЖЕНИЯ"].Kind != KindDivider {
		t.Error("10. ПРИЛОЖЕНИЯ должна быть KindDivider")
	}
	if byName["60. ГРАФИЧЕСКАЯ ЧАСТЬ"].Kind != KindDivider {
		t.Error("60. ГРАФИЧЕСКАЯ ЧАСТЬ должна быть KindDivider")
	}

	// Приложения — KindAppendix с буквами
	if byName["11. Первое"].Kind != KindAppendix {
		t.Error("11. Первое должна быть KindAppendix")
	}
	if byName["11. Первое"].Letter != "А" {
		t.Errorf("первое приложение: letter=%q, want А", byName["11. Первое"].Letter)
	}
	if byName["12. Второе"].Letter != "Б" {
		t.Errorf("второе приложение: letter=%q, want Б", byName["12. Второе"].Letter)
	}

	// После графической части — обычные
	if byName["61. Чертёж"].Kind != KindNormal {
		t.Error("61. Чертёж после ГРАФИЧЕСКАЯ ЧАСТЬ должна быть KindNormal")
	}

	// BookTitle приложений
	want := "Приложение А. Первое"
	if byName["11. Первое"].BookTitle != want {
		t.Errorf("BookTitle = %q, want %q", byName["11. Первое"].BookTitle, want)
	}
}

func TestCollectEntriesNoMarkers(t *testing.T) {
	// appendix=true но маркеров нет — все PDF как KindNormal
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1. Документ.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2. Ещё.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := CollectEntries(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Kind != KindNormal {
			t.Errorf("без маркеров все должны быть KindNormal, got %v for %q", e.Kind, e.RawName)
		}
	}
}

func TestDividerBookTitle(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "10. ПРИЛОЖЕНИЯ"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := CollectEntries(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	// BookTitle заглушки — uppercase имя
	if entries[0].BookTitle != "ПРИЛОЖЕНИЯ" {
		t.Errorf("BookTitle = %q, want ПРИЛОЖЕНИЯ", entries[0].BookTitle)
	}
}
