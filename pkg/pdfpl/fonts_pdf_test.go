package pdfpl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	model "github.com/oliverpool/unipdf/v3/model"
)

func TestLogicalFor(t *testing.T) {
	cases := map[string]string{
		"TimesNewRomanPSMT":      "TimesNewRomanPSMT",
		"TimesNewRomanPS-BoldMT": "TimesNewRomanPS-BoldMT",
		"Helvetica":              "Helvetica",
		"DECAFE+SomeFont-Bold":   "SomeFont-Bold",
		"AB12C+MyFont-Regular":   "MyFont-Regular",
		"":                       "",
	}
	for base, want := range cases {
		if got := logicalFor(base); got != want {
			t.Errorf("logicalFor(%q) = %q, want %q", base, got, want)
		}
	}
}

func TestFontNorm(t *testing.T) {
	cases := map[string]string{
		"XO_Thames_Bu.ttf": "xothamesbuttf",
		"XOThames-Bold":    "xothamesbold",
		"Times  New!":      "timesnew",
	}
	for in, want := range cases {
		if got := fontNorm(in); got != want {
			t.Errorf("fontNorm(%q) = %q, want %q", in, got, want)
		}
	}
}

// pickSystemFont возвращает путь к любому системному TrueType или пропускает
// тест, если шрифтов на машине нет (CI без них).
func pickSystemFont(t *testing.T) string {
	t.Helper()
	for _, d := range systemFontDirs() {
		ents, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, ent := range ents {
			if ent.IsDir() {
				continue
			}
			low := strings.ToLower(ent.Name())
			if strings.HasSuffix(low, ".ttf") || strings.HasSuffix(low, ".otf") {
				return filepath.Join(d, ent.Name())
			}
		}
	}
	t.Skip("системные TrueType-шрифты не найдены")
	return ""
}

func TestSystemFontDirs(t *testing.T) {
	t.Logf("каталогов шрифтов: %d", len(systemFontDirs()))
}

func TestCoveredByCompositeFont(t *testing.T) {
	ttf := pickSystemFont(t)
	fn, err := model.NewCompositePdfFontFromTTFFile(ttf)
	if err != nil {
		t.Fatalf("NewCompositePdfFontFromTTFFile(%s): %v", ttf, err)
	}
	if !isComposite(fn) {
		t.Error("isComposite(fn) = false, want true")
	}
	// Внедрённый полный TTF даёт cmap со всеми глифами Тима и кириллицы —
	// составной шрифт покрывает оба текста. Пропуск, если у шрифта нет
	// FontFile2 и parseFontCmap вернул nil — тогда покрытия нет.
	if cov := fontCmap(fn); cov == nil {
		t.Logf("пропуск: внедрённой программы шрифта нет (%s)", ttf)
		return
	}
	for _, s := range []string{"Hello", "Привет"} {
		if !coveredBy(fn, s) {
			t.Errorf("coveredBy(%q) = false, want true (все руны есть в cmap)", s)
		}
	}
}

func TestCoveredBySimpleFont(t *testing.T) {
	// WinAnsi-шрифт (Times-Roman стандарт 14) — как в плейсхолдер-ранах PDF:
	// ASCII покрыт, кириллица — нет.
	fn, err := model.NewStandard14Font(model.TimesRomanName)
	if err != nil {
		t.Fatalf("NewStandard14Font: %v", err)
	}
	if !coveredBy(fn, "Hello 123") {
		t.Error("coveredBy(\"Hello 123\") = false, want true")
	}
	if coveredBy(fn, "Привет") {
		t.Error("coveredBy(\"Привет\") = true, want false (WinAnsi не знает кириллицу)")
	}
	if isComposite(fn) {
		t.Error("isComposite(Times-Roman) = true, want false")
	}
}

func TestIsSubsetFontName(t *testing.T) {
	cases := map[string]bool{
		"":                        false,
		"TimesNewRomanPSMT":       false,
		"BCDEEE+XOThames-Regular": true,
		"BCDFEE+XOThames-Bold":    true,
		"ABC+Times-Roman":         true,
	}
	for base, want := range cases {
		if got := isSubsetFontName(base); got != want {
			t.Errorf("isSubsetFontName(%q) = %v, want %v", base, got, want)
		}
	}
}

func TestEmbeddedCodecThroughManager(t *testing.T) {
	m := newFontManager()
	_, embed, err := m.fontForLogical("TimesNewRomanPSMT", "Привет")
	if err != nil {
		t.Skipf("системный шрифт не найден: %v", err)
	}

	orig, err := model.NewStandard14Font(model.TimesRomanName)
	if err != nil {
		t.Fatalf("orig font: %v", err)
	}
	c := &embeddedCodec{orig: orig, embed: embed, fam: "TimesNewRomanPSMT", m: m, key: "/TF1"}

	if !c.Hex() {
		t.Error("Hex() = false, want true (Identity-H)")
	}
	enc := c.Encode("Привет")
	if enc == "" {
		t.Error("Encode(\"Привет\") пустой")
	}
	t.Logf("Encode(\"Привет\") = %d байт CIDs", len(enc))

	// Width() — метрики ИСХОДНОГО шрифта; применимы к плейсхолдер-тексту (ASCII).
	if w := c.Width("Name"); w <= 0 {
		t.Errorf("Width(\"Name\") = %g, want > 0", w)
	}
	if w := c.NewWidth("Привет"); w <= 0 {
		t.Errorf("NewWidth(\"Привет\") = %g, want > 0", w)
	}
	if c.FontName() != "/TF1" {
		t.Errorf("FontName() = %q, want /TF1", c.FontName())
	}
}
