package appendixutil

import "testing"

func TestLetter(t *testing.T) {
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
		{50, "БА"},
	}
	for _, c := range cases {
		got := Letter(c.n)
		if got != c.want {
			t.Errorf("Letter(%d) = %q, want %q", c.n, got, c.want)
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
		{"3. Содержание", "Содержание"},
	}
	for _, c := range cases {
		got := CleanFileName(c.in)
		if got != c.want {
			t.Errorf("CleanFileName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsMarker(t *testing.T) {
	if !IsMarker("10. ПРИЛОЖЕНИЯ", MarkerAppendixBegin) {
		t.Error("10. ПРИЛОЖЕНИЯ должно быть маркером начала")
	}
	if !IsMarker("60. ГРАФИЧЕСКАЯ ЧАСТЬ", MarkerAppendixEnd) {
		t.Error("60. ГРАФИЧЕСКАЯ ЧАСТЬ должно быть маркером конца")
	}
	if IsMarker("5. Текстовая часть", MarkerAppendixBegin) {
		t.Error("5. Текстовая часть не должна быть маркером")
	}
	// Нижний регистр тоже работает
	if !IsMarker("приложения", MarkerAppendixBegin) {
		t.Error("нижний регистр должен работать")
	}
}

func TestLetterSequenceNoSkips(t *testing.T) {
	// Проверяем что З(3), Й(9), О(14), Ч(19), Ъ(21), Ы(22), Ь(23) отсутствуют
	forbidden := map[string]bool{"З": true, "Й": true, "О": true, "Ч": true, "Ъ": true, "Ы": true, "Ь": true, "Ё": true}
	for i := range Letters {
		if forbidden[Letters[i]] {
			t.Errorf("буква %q не должна быть в списке по ГОСТ", Letters[i])
		}
	}
}
