package fontutil

import (
	"encoding/binary"
	"testing"

	core "github.com/oliverpool/unipdf/v3/core"
	"github.com/oliverpool/unipdf/v3/model"
)

// Регрессия на баг unipdf makeCIDWidthArr: /W собирается по границам GID, и
// диапазон затирает ширины глифов, попавших внутрь. Именно это давало
// «наезд букв» в Acrobat, хотя PyMuPDF показывал верную картину по hmtx.

const (
	testUpem = 2048 // как у Arial
)

// buildTTF собирает минимальный, но валидный по структуре TrueType с таблицами
// head/hhea/maxp/hmtx. Нужен ровно настолько, чтобы разбирался hmtx.
func buildTTF(adv []uint16) []byte {
	numGlyphs := len(adv)
	numHM := numGlyphs

	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:20], testUpem)

	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[34:36], uint16(numHM))

	maxp := make([]byte, 32)
	binary.BigEndian.PutUint32(maxp[0:4], 0x00010000)
	binary.BigEndian.PutUint16(maxp[4:6], uint16(numGlyphs))

	hmtx := make([]byte, 4*numHM)
	for g := range numHM {
		binary.BigEndian.PutUint16(hmtx[4*g:4*g+2], adv[g])
	}

	tables := []struct {
		tag  string
		data []byte
	}{
		{"head", head},
		{"hhea", hhea},
		{"maxp", maxp},
		{"hmtx", hmtx},
	}

	const dirHeader = 12
	const dirEntry = 16
	buf := make([]byte, dirHeader+dirEntry*len(tables))
	binary.BigEndian.PutUint32(buf[0:4], 0x00010000)
	binary.BigEndian.PutUint16(buf[4:6], uint16(len(tables)))
	off := dirHeader + dirEntry*len(tables)
	for i, tb := range tables {
		e := dirHeader + dirEntry*i
		copy(buf[e:e+4], tb.tag)
		binary.BigEndian.PutUint32(buf[e+8:e+12], uint32(off))
		binary.BigEndian.PutUint32(buf[e+12:e+16], uint32(len(tb.data)))
		buf = append(buf, tb.data...)
		off += len(tb.data)
	}
	return buf
}

// pageWithFont собирает страницу с одним составным шрифтом Font2 и заранее
// испорченным /W — ровно то состояние, которое оставляет unipdf.
func pageWithFont(t *testing.T, adv []uint16, cidToGID core.PdfObject) (*model.PdfPage, *core.PdfObjectDictionary) {
	t.Helper()

	ff, err := core.MakeStream(buildTTF(adv), nil)
	if err != nil {
		t.Fatalf("MakeStream: %v", err)
	}
	desc := core.MakeDict()
	desc.Set("FontName", core.MakeName("TestFont"))
	desc.Set("FontFile2", core.MakeIndirectObject(ff))

	cid := core.MakeDict()
	cid.Set("Subtype", core.MakeName("CIDFontType2"))
	cid.Set("BaseFont", core.MakeName("TestFont"))
	cid.Set("CIDSystemInfo", core.MakeDict())
	cid.Set("FontDescriptor", core.MakeIndirectObject(desc))
	if cidToGID != nil {
		cid.Set("CIDToGIDMap", cidToGID)
	}
	cid.Set("DW", core.MakeFloat(750))
	// Сломанный /W: сплошной диапазон на весь шрифт с чужой шириной.
	brokenW := core.MakeArray()
	brokenW.Append(core.MakeInteger(0))
	brokenW.Append(core.MakeInteger(int64(len(adv) - 1)))
	brokenW.Append(core.MakeInteger(333))
	cid.Set("W", brokenW)

	descendants := core.MakeArray()
	descendants.Append(core.MakeIndirectObject(cid))

	type0 := core.MakeDict()
	type0.Set("Subtype", core.MakeName("Type0"))
	type0.Set("BaseFont", core.MakeName("TestFont"))
	type0.Set("Encoding", core.MakeName("Identity-H"))
	type0.Set("DescendantFonts", descendants)

	page := model.NewPdfPage()
	page.Resources = model.NewPdfPageResources()
	if err := page.Resources.SetFontByName("Font2", type0); err != nil {
		t.Fatalf("SetFontByName: %v", err)
	}
	return page, cid
}

// widthsFromW раскладывает /W в карту CID -> ширина.
func widthsFromW(t *testing.T, arr *core.PdfObjectArray) map[int]int {
	t.Helper()
	out := make(map[int]int)
	for i := 0; i+2 < arr.Len(); i += 3 {
		c1, ok1 := core.GetIntVal(arr.Get(i))
		c2, ok2 := core.GetIntVal(arr.Get(i + 1))
		w, ok3 := core.GetIntVal(arr.Get(i + 2))
		if !ok1 || !ok2 || !ok3 {
			t.Fatalf("строение /W сломалось на элементе %d", i)
		}
		for c := c1; c <= c2; c++ {
			out[c] = w
		}
	}
	return out
}

// expectWidth переводит аванс из единиц шрифта в тысячные доли em так же, как
// это делает Acrobat при чтении /W.
func expectWidth(units uint16) int {
	return int(float64(units)*1000/testUpem + 0.5)
}

func TestParseTTFAdvancesScaling(t *testing.T) {
	adv := []uint16{718, 669, 556, 1396, 722}
	a, err := parseTTFAdvances(buildTTF(adv))
	if err != nil {
		t.Fatalf("parseTTFAdvances: %v", err)
	}
	if a.numGlyphs != len(adv) {
		t.Errorf("numGlyphs = %d, ждали %d", a.numGlyphs, len(adv))
	}
	for g, want := range adv {
		if got := a.width(g); got != expectWidth(want) {
			t.Errorf("width(%d) = %d, ждали %d", g, got, expectWidth(want))
		}
	}
}

func TestParseTTFAdvancesMissingFullMetrics(t *testing.T) {
	// numberOfHMetrics меньше numGlyphs: последние глифы повторяют аванс
	// последней записи hmtx (правило спецификации TrueType).
	adv := []uint16{718, 669, 556}
	ttf := buildTTF(adv)
	// Переписываем hhea.numberOfHMetrics в 2 прямо в собранном файле.
	// Каталог: 12 + 16*4 = 76; hhea — второй по алфавиту таблиц после head.
	hheaOff := int(binary.BigEndian.Uint32(ttf[76+16+8 : 76+16+12]))
	binary.BigEndian.PutUint16(ttf[hheaOff+34:hheaOff+36], 2)

	a, err := parseTTFAdvances(ttf)
	if err != nil {
		t.Fatalf("parseTTFAdvances: %v", err)
	}
	if got := a.width(2); got != expectWidth(556) {
		t.Errorf("глиф за пределами numberOfHMetrics: width(2) = %d, ждали %d (повтор последнего)",
			got, expectWidth(556))
	}
}

func TestParseTTFAdvancesRejectsGarbage(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"пусто", nil},
		{"обрезанный заголовок", []byte{0x00, 0x01}},
		{"мусор вместо каталога", []byte("не шрифт, а текст подлиннее...")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseTTFAdvances(tc.data); err == nil {
				t.Fatal("ожидалась ошибка разбора, получена nil")
			}
		})
	}
}

// Главная регрессия: диапазон /W не должен накрывать глифы с другой шириной.
func TestRepairPageKeepsNeighbourWidths(t *testing.T) {
	// Ширина 354 в тысячных, а соседние глифы заметно шире — ровно та
	// ситуация, где unipdf склеивал диапазон по GID.
	wide := uint16(718 * testUpem / 1000)
	narrow := uint16(354 * testUpem / 1000)
	mid := uint16(556 * testUpem / 1000)
	adv := []uint16{wide, narrow, narrow, narrow, mid}

	page, cid := pageWithFont(t, adv, core.MakeName("Identity"))
	r := NewCidWidthRepair()
	n, err := r.RepairPage(page)
	if err != nil {
		t.Fatalf("RepairPage: %v", err)
	}
	if n != 1 {
		t.Fatalf("починено %d шрифтов, ждали 1", n)
	}

	arr, ok := core.GetArray(cid.Get("W"))
	if !ok {
		t.Fatalf("/W не массив: %v", cid.Get("W"))
	}
	got := widthsFromW(t, arr)
	if len(got) != len(adv) {
		t.Fatalf("/W покрывает %d глифов, ждали %d", len(got), len(adv))
	}
	for g, want := range adv {
		if got[g] != expectWidth(want) {
			t.Errorf("CID %d: /W = %d, ждали %d", g, got[g], expectWidth(want))
		}
	}

	// Отдельно фиксируем главный симптом: широкий глиф не затирается узким.
	if got[0] == got[1] {
		t.Errorf("глиф 0 и глиф 1 получили одинаковую ширину %d — диапазон снова накрыл соседей",
			got[0])
	}
	if dw, ok := core.GetFloatVal(cid.Get("DW")); !ok || dw <= 0 {
		t.Errorf("/DW = %v, ждали положительную запасную ширину", cid.Get("DW"))
	}
}

func TestRepairPageSkipsNonIdentityCIDToGIDMap(t *testing.T) {
	// Без CIDToGIDMap /Identity индекс /W — это не GID, трогать нельзя.
	adv := []uint16{718, 354, 556}
	page, cid := pageWithFont(t, adv, core.MakeName("Custom"))
	before, _ := core.GetArray(cid.Get("W"))

	n, err := NewCidWidthRepair().RepairPage(page)
	if err != nil {
		t.Fatalf("RepairPage: %v", err)
	}
	if n != 0 {
		t.Errorf("шрифт с не-Identity маппингом починен (%d), ждали 0", n)
	}
	after, _ := core.GetArray(cid.Get("W"))
	if before.Len() != after.Len() {
		t.Errorf("/W изменён: было %d элементов, стало %d", before.Len(), after.Len())
	}
}

func TestRepairPageSkipsSimpleFont(t *testing.T) {
	page := model.NewPdfPage()
	page.Resources = model.NewPdfPageResources()
	simple := core.MakeDict()
	simple.Set("Subtype", core.MakeName("TrueType"))
	simple.Set("BaseFont", core.MakeName("Helvetica"))
	// Простой шрифт с настоящими /Widths: unipdf тут не ошибается.
	simple.Set("FirstChar", core.MakeInteger(32))
	simple.Set("LastChar", core.MakeInteger(33))
	widths := core.MakeArray()
	widths.Append(core.MakeInteger(278))
	widths.Append(core.MakeInteger(278))
	simple.Set("Widths", widths)
	if err := page.Resources.SetFontByName("F1", simple); err != nil {
		t.Fatalf("SetFontByName: %v", err)
	}

	n, err := NewCidWidthRepair().RepairPage(page)
	if err != nil {
		t.Fatalf("RepairPage: %v", err)
	}
	if n != 0 {
		t.Errorf("простой шрифт починен (%d), ждали 0", n)
	}
}

func TestRepairPageCachesFontFile(t *testing.T) {
	// Один и тот же объект шрифта на нескольких страницах: FontFile2 должен
	// разбираться один раз, иначе том с 1 МБ Arial читается сотни раз.
	adv := []uint16{718, 354, 556}
	page1, _ := pageWithFont(t, adv, core.MakeName("Identity"))
	page2, _ := pageWithFont(t, adv, core.MakeName("Identity"))
	// Кладём на обе страницы один и тот же объект шрифта.
	res1, _ := core.GetDict(page1.Resources.Font)
	res2, _ := core.GetDict(page2.Resources.Font)
	res2.Set("Font2", res1.Get("Font2"))

	r := NewCidWidthRepair()
	for _, page := range []*model.PdfPage{page1, page2} {
		if _, err := r.RepairPage(page); err != nil {
			t.Fatalf("RepairPage: %v", err)
		}
	}
	if len(r.parsed) != 1 {
		t.Errorf("разобранo %d разных FontFile2, ждали 1 (кэш)", len(r.parsed))
	}
}
