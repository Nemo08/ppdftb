package fontutil

import (
	"encoding/binary"
	"fmt"
	"log/slog"

	"github.com/oliverpool/unipdf/v3/core"
	"github.com/oliverpool/unipdf/v3/model"
)

// Ремонт метрик составных шрифтов (/W, /DW).
//
// Зачем это нужно. unipdf строит /W составного шрифта в makeCIDWidthArr: он
// берёт руны, отсортированные по Unicode, склеивает подряд идущие руны с
// одинаковой шириной в один диапазон и записывает его границы ПО GID
// («c_first c_last w», PDF 32000 9.7.4.3). Такое склеивание корректно только
// если GID идут по рунам строго по возрастанию и без пропусков. В реальных
// шрифтах (Arial) это не так: между GID первой и последней руны диапазона
// лежат глифы ДРУГИХ рун, и unipdf затирает их ширины одним значением.
//
// Последствия видны только в Acrobat. Ширины глифов Acrobat берёт из /W, а не
// из внедрённого hmtx, поэтому глиф получает заниженный аванс: заглавные
// слипаются, строчные наоборот расходятся. PyMuPDF, наоборот, меряет по
// внедрённому hmtx и показывает «правильную» ширину — поэтому на его рендере
// дефект не виден, и пересчёт ширины в коде (numberMetrics.width) даёт верную
// геометрию, не совпадающую с Acrobat.
//
// Тот же дефект шаблонизатор чинит в FontManager.SubsetAll — там /W
// перекрывается вручную после ToPdfObject. Здесь тот же приём для шрифтов
// нумерации страниц, которые идут через creator в обход FontManager.
//
// Разбор разбора и метода лечения — в README.md этого пакета.

// ttfAdvances — метрики внедрённого TrueType: аванс каждого глифа в единицах
// шрифта плюс коэффициент перевода в тысячные доли em.
type ttfAdvances struct {
	advances  []uint16
	numGlyphs int
	k         float64 // 1000 / unitsPerEm
}

// CidWidthRepair пересчитывает /W и /DW составных шрифтов страниц по
// метрикам внедрённых FontFile2.
//
// Разобранные шрифты кэшируются: один и тот же объект шрифта входит в
// ресурсы десятков страниц, а файл Arial — 1 МБ.
type CidWidthRepair struct {
	parsed map[core.PdfObject]*ttfAdvances
}

// NewCidWidthRepair создаёт ремонт метрик с кэшем разобранных шрифтов.
func NewCidWidthRepair() *CidWidthRepair {
	return &CidWidthRepair{parsed: make(map[core.PdfObject]*ttfAdvances)}
}

// RepairPage пересчитывает метрики всех составных шрифтов ресурсов страницы и
// возвращает число починенных шрифтов. Простые шрифты (TrueType/Type1 с
// /Widths) не трогает: там unipdf ключу кода соответствует ширина.
func (r *CidWidthRepair) RepairPage(page *model.PdfPage) (int, error) {
	if page == nil || page.Resources == nil {
		return 0, nil
	}
	fonts, ok := core.GetDict(page.Resources.Font)
	if !ok {
		return 0, nil
	}
	repaired := 0
	for _, name := range fonts.Keys() {
		n, err := r.repairFont(fonts.Get(name))
		if err != nil {
			return repaired, err
		}
		repaired += n
	}
	return repaired, nil
}

// repairFont пересчитывает метрики одного шрифта. Возвращает 1, если шрифт
// был составным CIDFontType2 с внедрённым FontFile2 и метрики пересчитаны.
func (r *CidWidthRepair) repairFont(obj core.PdfObject) (int, error) {
	type0, ok := core.GetDict(core.TraceToDirectObject(obj))
	if !ok {
		return 0, nil
	}
	if st, ok := type0.Get("Subtype").(*core.PdfObjectName); !ok || string(*st) != "Type0" {
		return 0, nil
	}
	cid := cidFontDictOf(type0)
	if cid == nil {
		return 0, nil
	}
	// Пересчитываем только при CIDToGIDMap /Identity: тогда индекс /W — это
	// ровно GID, который мы читаем из hmtx. При /CIDToGIDStream или отсутствии
	// маппинга соответствия нет, и трогать /W нельзя.
	if cm, ok := cid.Get("CIDToGIDMap").(*core.PdfObjectName); ok && string(*cm) != "Identity" {
		return 0, nil
	}
	adv, err := r.advancesOf(cid)
	if err != nil {
		return 0, err
	}
	if adv == nil {
		return 0, nil
	}
	cid.Set("W", adv.widthArray())
	cid.Set("DW", core.MakeFloat(adv.defaultWidth()))
	return 1, nil
}

// advancesOf достаёт метрики FontFile2 потомка шрифта с кэшем по ссылке на
// встроенный файл. nil означает «шрифт нечего чинить».
func (r *CidWidthRepair) advancesOf(cid *core.PdfObjectDictionary) (*ttfAdvances, error) {
	desc, ok := core.GetDict(core.TraceToDirectObject(cid.Get("FontDescriptor")))
	if !ok {
		return nil, nil
	}
	ref := desc.Get("FontFile2")
	if a, ok := r.parsed[ref]; ok {
		return a, nil
	}
	st, ok := core.GetStream(core.TraceToDirectObject(ref))
	if !ok {
		return nil, nil
	}
	// Поток из файла приходит сжатым (обычно FlateDecode) — парсеру hmtx нужен
	// распакованный байтовый код TrueType.
	data, err := core.DecodeStream(st)
	if err != nil {
		slog.Debug("fontutil: не удалось распаковать FontFile2, /W оставлен как есть",
			slog.String("error", err.Error()))
		r.parsed[ref] = nil
		return nil, nil
	}
	a, err := parseTTFAdvances(data)
	if err != nil {
		// Шрифт не читается — оставляем /W как есть: лучше метрики unipdf,
		// чем никаких.
		slog.Debug("fontutil: не удалось разобрать FontFile2, /W оставлен как есть",
			slog.String("error", err.Error()))
		r.parsed[ref] = nil
		return nil, nil
	}
	r.parsed[ref] = a
	return a, nil
}

// width возвращает аванс глифа в тысячных долях em.
func (a *ttfAdvances) width(gid int) int {
	if gid < 0 || gid >= len(a.advances) {
		return 0
	}
	return int(float64(a.advances[gid])*a.k + 0.5)
}

// cidWidthsFromArray раскладывает /W в карту CID → ширина. Поддерживает обе
// формы записи из PDF 32000 9.7.4.3: «c w» и «c_first c_last w».
func cidWidthsFromArray(arr *core.PdfObjectArray) map[int]int {
	out := make(map[int]int, arr.Len())
	for i := 0; i+1 < arr.Len(); {
		c1, ok1 := core.GetIntVal(arr.Get(i))
		if !ok1 {
			break
		}
		// «c_first c_last w»
		if c2, ok2 := core.GetIntVal(arr.Get(i + 1)); ok2 && i+2 < arr.Len() {
			if w, ok3 := core.GetIntVal(arr.Get(i + 2)); ok3 {
				for c := c1; c <= c2; c++ {
					out[c] = w
				}
				i += 3
				continue
			}
		}
		// «c w»
		if w, ok2 := core.GetIntVal(arr.Get(i + 1)); ok2 {
			out[c1] = w
			i += 2
			continue
		}
		break
	}
	return out
}

// CountBrokenOnPage считает по всем составным шрифтам страницы расхождения между
// /W и авансами глифов во внедрённом FontFile2. 0 означает, что метрики
// корректны.
//
// Проверка нужна, потому что Acrobat берёт ширины глифов из /W, а не из
// внедрённого hmtx: расхождение пользователю видно как наезд букв, тогда как
// рендерилки на основе hmtx показывают корректную картину.
func (r *CidWidthRepair) CountBrokenOnPage(page *model.PdfPage) (int, error) {
	if page == nil || page.Resources == nil {
		return 0, nil
	}
	fonts, ok := core.GetDict(page.Resources.Font)
	if !ok {
		return 0, nil
	}
	broken := 0
	for _, name := range fonts.Keys() {
		type0, ok := core.GetDict(core.TraceToDirectObject(fonts.Get(name)))
		if !ok {
			continue
		}
		if st, ok := type0.Get("Subtype").(*core.PdfObjectName); !ok || string(*st) != "Type0" {
			continue
		}
		cid := cidFontDictOf(type0)
		if cid == nil {
			continue
		}
		if cm, ok := cid.Get("CIDToGIDMap").(*core.PdfObjectName); ok && string(*cm) != "Identity" {
			continue
		}
		adv, err := r.advancesOf(cid)
		if err != nil || adv == nil {
			continue
		}
		arr, ok := core.GetArray(core.TraceToDirectObject(cid.Get("W")))
		if !ok {
			broken++
			continue
		}
		for cidNum, w := range cidWidthsFromArray(arr) {
			if w != adv.width(cidNum) {
				broken++
			}
		}
	}
	return broken, nil
}

// widthArray собирает /W в форме «c_first c_last w» по истинным авансам.
// Ключ — GID (CIDToGIDMap /Identity). Соседние глифы с одинаковой шириной
// сливаются в диапазон, но только реальные соседи: в отличие от unipdf здесь
// диапазон не перекрывает глифы с другой шириной.
func (a *ttfAdvances) widthArray() core.PdfObject {
	arr := core.MakeArray()
	for start := 0; start < a.numGlyphs; {
		w := a.width(start)
		end := start
		for end+1 < a.numGlyphs && a.width(end+1) == w {
			end++
		}
		arr.Append(core.MakeInteger(int64(start)))
		arr.Append(core.MakeInteger(int64(end)))
		arr.Append(core.MakeInteger(int64(w)))
		start = end + 1
	}
	return arr
}

// defaultWidth возвращает /DW — запасную ширину для CID вне /W. Берётся самый
// частый аванс глифов (мода), а не ноль: иначе неизвестный глиф рисуется
// невидимым и налезает на соседние.
func (a *ttfAdvances) defaultWidth() float64 {
	counts := make(map[int]int, a.numGlyphs)
	for g := range a.numGlyphs {
		counts[a.width(g)]++
	}
	best, bestN := 0, 0
	for g := range a.numGlyphs {
		if w := a.width(g); counts[w] > bestN {
			best, bestN = w, counts[w]
		}
	}
	return float64(best)
}

// parseTTFAdvances читает из TrueType таблицы head/hhea/maxp/hmtx и строит
// авансы всех глифов. Собственный разбор вместо готовой библиотеки: нужен
// только hmtx, а unidoc/unitype держит нужные поля неэкспортируемыми.
func parseTTFAdvances(data []byte) (*ttfAdvances, error) {
	const dirHeader = 12
	const dirEntry = 16
	if len(data) < dirHeader {
		return nil, fmt.Errorf("fontutil: TrueType короче заголовка (%d байт)", len(data))
	}
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	if dirHeader+dirEntry*numTables > len(data) {
		return nil, fmt.Errorf("fontutil: каталог таблиц TrueType обрезан (%d таблиц)", numTables)
	}
	tables := make(map[string][]byte, numTables)
	for i := range numTables {
		off := dirHeader + dirEntry*i
		tag := string(data[off : off+4])
		start := int(binary.BigEndian.Uint32(data[off+8 : off+12]))
		length := int(binary.BigEndian.Uint32(data[off+12 : off+16]))
		if start < 0 || length < 0 || start > len(data) || start+length > len(data) {
			continue
		}
		tables[tag] = data[start : start+length]
	}
	head, hhea, maxp, hmtx := tables["head"], tables["hhea"], tables["maxp"], tables["hmtx"]
	if len(head) < 20 || len(hhea) < 36 || len(maxp) < 6 || len(hmtx) < 4 {
		return nil, fmt.Errorf("fontutil: в TrueType нет таблиц head/hhea/maxp/hmtx")
	}
	upem := int(binary.BigEndian.Uint16(head[18:20]))
	if upem == 0 {
		return nil, fmt.Errorf("fontutil: unitsPerEm = 0")
	}
	numGlyphs := int(binary.BigEndian.Uint16(maxp[4:6]))
	numHM := int(binary.BigEndian.Uint16(hhea[34:36]))
	if numGlyphs == 0 {
		return nil, fmt.Errorf("fontutil: maxp.numGlyphs = 0")
	}
	if numHM == 0 || numHM > numGlyphs || 4*numHM > len(hmtx) {
		return nil, fmt.Errorf("fontutil: hhea.numberOfHMetrics=%d не согласуется с maxp.numGlyphs=%d", numHM, numGlyphs)
	}
	adv := make([]uint16, numGlyphs)
	for g := range numHM {
		adv[g] = binary.BigEndian.Uint16(hmtx[4*g : 4*g+2])
	}
	// Глифы с индексами от numberOfHMetrics не имеют записи аванса и по
	// спецификации повторяют последнюю из hmtx.
	for g := numHM; g < numGlyphs; g++ {
		adv[g] = adv[numHM-1]
	}
	return &ttfAdvances{
		advances:  adv,
		numGlyphs: numGlyphs,
		k:         1000 / float64(upem),
	}, nil
}
