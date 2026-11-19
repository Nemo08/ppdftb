package fontutil

import "testing"

func TestApproxRuneWidth(t *testing.T) {
	cases := []struct {
		r    rune
		want float64
	}{
		{'1', 278}, // узкая цифра
		{'7', 500}, // обычная цифра
		{'I', 278}, // узкая заглавная
		{'A', 650}, // широкая заглавная
		{'a', 500}, // строчная
		{'.', 278}, // узкий знак
		{'{', 500}, // скобка плейсхолдера
		{' ', 278}, // пробел
		{'Ж', 500}, // неизвестный символ — по умолчанию
	}
	for _, c := range cases {
		if got := approxRuneWidth(c.r); got != c.want {
			t.Errorf("approxRuneWidth(%q) = %v, want %v", c.r, got, c.want)
		}
	}
}

func TestEstimateWidth(t *testing.T) {
	cases := map[string]float64{
		"":       0,
		"1":      278,     // узкая цифра
		"2":      500,     // обычная цифра
		"12":     778,     // 278 + 500
		". ":     556,     // 278 + 278
		"A":      650,     // широкая заглавная
		"Привет": 500 * 6, // кириллица — по умолчанию
	}
	for in, want := range cases {
		if got := EstimateWidth(in); got != want {
			t.Errorf("EstimateWidth(%q) = %v, want %v", in, got, want)
		}
	}
}
