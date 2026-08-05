package pdf

import (
	"testing"
)

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
