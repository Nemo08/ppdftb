package pdfpl

import (
	"strings"
	"testing"

	model "github.com/oliverpool/unipdf/v3/model"
)

func TestFamilyFor(t *testing.T) {
	cases := map[string]string{
		"TimesNewRomanPSMT":       "times.ttf",
		"TimesNewRomanPS-BoldMT":  "timesbd.ttf",
		"Helvetica":               "arial.ttf",
		"Helvetica-Bold":          "arialbd.ttf",
		"ArialMT":                 "arial.ttf",
		"Calibri":                 "arial.ttf",
		"Calibri-Bold":            "arialbd.ttf",
		"BCDEEE+XOThames-Regular": "times.ttf",
		"BCDGEE+XOThames-Bold":    "timesbd.ttf",
		"COURIER":                 "times.ttf", // фолбэк
		"":                        "times.ttf",
	}
	for base, want := range cases {
		if got := familyFor(base); got != want {
			t.Errorf("familyFor(%q) = %q, want %q", base, got, want)
		}
	}
}

// pickSystemFont возвращает путь к системному TTF какого-нибудь семейства
// или пропускает тест, если шрифтов на машине нет (CI без них).
func pickSystemFont(t *testing.T) string {
	t.Helper()
	for _, fam := range []string{"times.ttf", "arial.ttf", "timesbd.ttf"} {
		if p := findSystemFont(fam); p != "" {
			return p
		}
	}
	t.Skip("системные TrueType-шрифты не найдены")
	return ""
}

func TestFindSystemFont(t *testing.T) {
	p := findSystemFont("times.ttf")
	if !strings.EqualFold(p, "") && !strings.HasSuffix(strings.ToLower(p), ".ttf") {
		t.Errorf("findSystemFont = %q, want путь до .ttf", p)
	}
	t.Logf("times: %q", p)
}

func TestCoveredByCompositeFont(t *testing.T) {
	ttf := pickSystemFont(t)
	fn, err := model.NewCompositePdfFontFromTTFFile(ttf)
	if err != nil {
		t.Fatalf("NewCompositePdfFontFromTTFFile(%s): %v", ttf, err)
	}
	// Составной шрифт (Identity-H) всегда считается непокрытым: его CID —
	// номера глифов, произвольный текст в него байтами не вставишь.
	for _, s := range []string{"Hello", "Привет"} {
		if coveredBy(fn, s) {
			t.Errorf("coveredBy(%q) = true, want false (составной шрифт)", s)
		}
	}
	if !isComposite(fn) {
		t.Error("isComposite(fn) = false, want true")
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
	embed, err := m.fontFor(familyFor("TimesNewRomanPSMT"))
	if err != nil {
		t.Fatalf("fontFor: %v", err)
	}

	orig, err := model.NewStandard14Font(model.TimesRomanName)
	if err != nil {
		t.Fatalf("orig font: %v", err)
	}
	c := &embeddedCodec{orig: orig, embed: embed, fam: familyFor("TimesNewRomanPSMT"), m: m, key: "/TF1"}

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
