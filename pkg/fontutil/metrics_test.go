package fontutil

import (
	"math"
	"path/filepath"
	"testing"

	model "github.com/oliverpool/unipdf/v3/model"
)

// resolveTestFont поднимает настоящий системный шрифт через Resolve —
// метрики тестируем только на реальном шрифте, на синтетическом объекте
// GetRuneMetrics нечего проверять.
func resolveTestFont(t *testing.T, text string) *model.PdfFont {
	t.Helper()
	src, base := pickLoadableFont(t, false)
	withFontDirs(t, []string{filepath.Dir(src)})
	f, _, err := Resolve(base, text)
	if err != nil {
		t.Fatalf("Resolve(%q, %q): %v", base, text, err)
	}
	return f
}

// TestAdvanceScalesWithFontSize — главный контракт Advance: ширина линейна по
// кеглю. Ошибка в масштабе уводила бы номерный блок за поле.
func TestAdvanceScalesWithFontSize(t *testing.T) {
	const text = "Приложение А. 12"
	f := resolveTestFont(t, text)

	const size = 12.0
	a1 := Advance(f, '1', size)
	a2 := Advance(f, '1', size*2)

	if a1 <= 0 {
		t.Fatalf("Advance('1') = %v, want > 0", a1)
	}
	if math.Abs(a2-2*a1) > 1e-9 {
		t.Errorf("Advance при двойном кегле = %v, want %v", a2, 2*a1)
	}
}

// TestAdvanceUnknownRuneUsesFontMissingWidth — руны нет в шрифте. unipdf
// отдаёт /MissingWidth с ok=true, поэтому ширина должна быть положительной и
// равной ответу самого шрифта. Регрессия: возврат нуля схлопнул бы блок.
func TestAdvanceUnknownRuneUsesFontMissingWidth(t *testing.T) {
	const size = 10.0
	f := resolveTestFont(t, "12")

	// 0x10FFFF никогда не встречается в шрифте.
	const missing = rune(0x10FFFF)

	got := Advance(f, missing, size)
	if got <= 0 {
		t.Fatalf("Advance(неизвестная руна) = %v, want > 0", got)
	}

	cm, ok := f.GetRuneMetrics(missing)
	if ok && cm.Wx > 0 {
		want := cm.Wx * size / 1000
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("Advance = %v, want /MissingWidth шрифта %v", got, want)
		}
		return
	}
	// У шрифта нет /MissingWidth: обязан сработать наш фолбэк.
	want := 500.0 * size / 1000
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("Advance = %v, want фолбэк %v", got, want)
	}
}

// TestAdvanceNilFont — nil-шрифт не должен паниковать: вызывающий код не
// обязан проверять результат Resolve на nil перед расчётом.
func TestAdvanceNilFont(t *testing.T) {
	const size = 10.0
	got := Advance(nil, '1', size)
	want := 500.0 * size / 1000
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("Advance(nil, '1') = %v, want %v", got, want)
	}
}

// TestStringWidthIsSumOfAdvances — суммарная ширина равна сумме авансов.
func TestStringWidthIsSumOfAdvances(t *testing.T) {
	const text = "Приложение А. 12"
	const size = 12.0
	f := resolveTestFont(t, text)

	var want float64
	for _, r := range text {
		want += Advance(f, r, size)
	}
	if got := StringWidth(f, text, size); math.Abs(got-want) > 1e-9 {
		t.Errorf("StringWidth = %v, want %v", got, want)
	}
}

// TestStringWidthEmptyAndPositive — пустая строка даёт ноль, непустая —
// строго положительную ширину. На этом стоит расчёт leftX блока.
func TestStringWidthEmptyAndPositive(t *testing.T) {
	const size = 12.0
	f := resolveTestFont(t, "12")

	if got := StringWidth(f, "", size); got != 0 {
		t.Errorf("StringWidth(\"\") = %v, want 0", got)
	}
	if got := StringWidth(f, "1", size); got <= 0 {
		t.Errorf("StringWidth(\"1\") = %v, want > 0", got)
	}
}

// TestStringWidthMonotonic — добавление руны не уменьшает ширину. Раньше
// ошибка со знаком в advance давала бы блок, «схлопывающийся» на длинных
// надписях вроде «Приложение А. 12».
func TestStringWidthMonotonic(t *testing.T) {
	const size = 12.0
	f := resolveTestFont(t, "12")

	prev := 0.0
	for _, s := range []string{"П", "Пр", "При", "Прил", "Прило"} {
		got := StringWidth(f, s, size)
		if got <= prev {
			t.Errorf("StringWidth(%q) = %v, want > %v (ширина должна расти)", s, got, prev)
		}
		prev = got
	}
}

// TestEstimateWidthStillHeuristic — развести два API: EstimateWidth остаётся
// эвристикой (шрифта нет) и НЕ обязан совпадать с точным StringWidth.
func TestEstimateWidthStillHeuristic(t *testing.T) {
	const size = 12.0
	const text = "Приложение А. 12"
	f := resolveTestFont(t, text)

	est := EstimateWidth(text) * size / 1000
	exact := StringWidth(f, text, size)
	if est == exact {
		t.Logf("EstimateWidth случайно совпал с точным (%v) — тест не различает API", est)
	}
	if est <= 0 || exact <= 0 {
		t.Errorf("ширины должны быть положительными: est=%v exact=%v", est, exact)
	}
}
