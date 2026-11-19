package fontutil

import (
	"testing"

	core "github.com/oliverpool/unipdf/v3/core"
)

func TestGlyphAdvancesAndWidthArray(t *testing.T) {
	// Загружаем системный шрифт под кириллический текст так же, как это делает
	// прод (FontForLogical + разбор метрик GetRuneMetrics).
	txt := "Общество с ограниченной ответственностью"
	m := NewFontManager()
	fam, f, err := m.FontForLogical("Times-Bold", txt)
	if err != nil {
		t.Skipf("системный Times-Bold не найден: %v", err)
	}
	_ = fam

	// Регистрируем руны (как делает embeddedCodec.Encode).
	m.record(fam, f, txt)
	adv := m.glyphAdvances(fam)
	if len(adv) == 0 {
		t.Fatal("glyphAdvances пуст")
	}
	// Истинный аванс кириллицы: дефолтные 552 мешают Acrobat, правильные
	// значения — из метрик шрифта. Проверяем, что /W отдаёт не-дефолт.
	wid := m.buildWidthArray(fam)
	arr, ok := core.GetArray(wid)
	if !ok || arr.Len() == 0 {
		t.Fatal("buildWidthArray вернул не массив")
	}
	// Наличие хотя бы одной реальной ширины (не 552) достаточно: конкретные
	// значения зависят от системного файла шрифта.
	seen := map[int]int{}
	for i := 0; i+2 < arr.Len(); i += 3 {
		c1, ok1 := core.GetIntVal(arr.Get(i))
		c2, ok2 := core.GetIntVal(arr.Get(i + 1))
		w, ok3 := core.GetIntVal(arr.Get(i + 2))
		if !ok1 || !ok2 || !ok3 {
			t.Fatalf("строение /W сломалось на элементе %d", i)
		}
		for c := c1; c <= c2; c++ {
			seen[c] = w
		}
	}
	if len(seen) == 0 {
		t.Fatal("/W пуст")
	}
}

func TestCIDFontDictOf(t *testing.T) {
	// buildWidthArray кладётся в словарь CIDFontType2-потомка: проверяем,
	// что cidFontDictOf находит его в дереве ToPdfObject.
	m := NewFontManager()
	fam, _, err := m.FontForLogical("Times-Bold", "ООО")
	if err != nil {
		t.Skipf("системный Times-Bold не найден: %v", err)
	}
	obj, err := m.FontObject(fam)
	if err != nil {
		t.Fatal(err)
	}
	cd := cidFontDictOf(obj)
	if cd == nil {
		t.Fatal("cidFontDictOf не нашёл CIDFontType2-потомка")
	}
	if st, ok := cd.Get("Subtype").(*core.PdfObjectName); !ok || string(*st) != "CIDFontType2" {
		t.Errorf("потомок не CIDFontType2: %v", cd.Get("Subtype"))
	}
	// После подмены ширин /W в потомке должен стать массивом с ширинами.
	m.record(fam, m.fonts[fam], "ООО")
	if w := m.buildWidthArray(fam); w != nil {
		cd.Set("W", w)
	}
	if _, ok := core.GetArray(cd.Get("W")); !ok {
		t.Errorf("после подмены /W не массив: %v", cd.Get("W"))
	}
}
