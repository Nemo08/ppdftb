package fontutil

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	core "github.com/oliverpool/unipdf/v3/core"
	model "github.com/oliverpool/unipdf/v3/model"
)

// FontManager кэширует внедряемые TrueType-шрифты (один на логическое имя на
// файл). Для каждого шрифта ведётся карта CID→rune (CID == GID Identity-H), на
// основе которой после обработки страниц строится ToUnicode CMap — без него
// экстрактор текста не может восстановить Unicode из гекс-литералов.
type FontManager struct {
	fonts  map[string]*model.PdfFont  // логическое имя шрифта → загруженный шрифт
	objs   map[string]core.PdfObject  // логическое имя → PDF-объект шрифта в ресурсах
	cmaps  map[string]map[uint16]rune // логическое имя → CID→rune
	cached []*model.PdfFont           // все загруженные — для SubsetRegistered()
}

// NewFontManager создаёт пустой менеджер внедряемых шрифтов.
func NewFontManager() *FontManager {
	return &FontManager{
		fonts: map[string]*model.PdfFont{},
		objs:  map[string]core.PdfObject{},
		cmaps: map[string]map[uint16]rune{},
	}
}

// FontForLogical загружает (с кэшем) системный TrueType под внедряемый текст:
// шрифт подбирается по логическому имени BaseFont (точное PS-имя) либо, если
// его нет — любой установленный шрифт, покрывающий текст (фолбек «что есть»).
// Кэш и ресурсное пространство имён используют логическое имя, а не файл:
// кодек привязан к исходному рану, а не к физическому файлу шрифта.
func (m *FontManager) FontForLogical(logical, text string) (string, *model.PdfFont, error) {
	if f, ok := m.fonts[logical]; ok {
		return logical, f, nil
	}
	ttf := resolveEmbeddedFile(logical, text)
	if ttf == "" {
		return "", nil, fmt.Errorf("fontutil: системный шрифт %q не найден", logical)
	}
	f, err := model.NewCompositePdfFontFromTTFFile(ttf)
	if err != nil {
		return "", nil, fmt.Errorf("fontutil: загрузка TrueType %s: %w", ttf, err)
	}
	m.fonts[logical] = f
	m.cached = append(m.cached, f)
	return logical, f, nil
}

// FontObject возвращает кэшированный PDF-объект шрифта логического имени: один
// на имя, чтобы ToUnicode-стрим (добавляемый в SubsetAll) попал в тот же
// объект, что ссылки ресурсов всех страниц.
func (m *FontManager) FontObject(fam string) (core.PdfObject, error) {
	if o, ok := m.objs[fam]; ok {
		return o, nil
	}
	_, f, err := m.FontForLogical(fam, "")
	if err != nil {
		return nil, err
	}
	o := f.ToPdfObject()
	m.objs[fam] = o
	return o, nil
}

// record фиксирует соответствие CID→rune по фактическим байтам кодирования
// (один символ кодируется отдельно — код стабилен и совпадает с GID).
func (m *FontManager) record(fam string, f *model.PdfFont, utf8 string) {
	mapping, ok := m.cmaps[fam]
	if !ok {
		mapping = map[uint16]rune{}
	}
	miss := 0
	for _, r := range utf8 {
		b, m0 := f.StringToCharcodeBytes(string(r))
		if m0 > 0 {
			miss += m0
		}
		if len(b) == 2 {
			mapping[uint16(b[0])<<8|uint16(b[1])] = r
		} else if len(b) == 1 {
			mapping[uint16(b[0])] = r
		}
	}
	if miss > 0 {
		slog.Warn("fontutil: символы не кодируются во внедряемом шрифте", slog.String("font", fam), slog.Int("miss", miss))
	}
	m.cmaps[fam] = mapping
}

// buildToUnicode строит поток CMap (CID→UTF-16BE) для семейства и подвешивает
// его ключом ToUnicode в PDF-объект внедряемого шрифта. Возвращает nil, если
// для семейства не было записано ни одного символа.
func (m *FontManager) buildToUnicode(fam string) (core.PdfObject, error) {
	mapping := m.cmaps[fam]
	if len(mapping) == 0 {
		return nil, nil
	}
	// Сортировка CID для детерминированного вывода.
	ids := make([]uint16, 0, len(mapping))
	for cid := range mapping {
		ids = append(ids, cid)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	var sb strings.Builder
	sb.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n")
	sb.WriteString("/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n")
	sb.WriteString("/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n")
	sb.WriteString("1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")

	// bfchar порциями не более 100 строк (ограничение синтаксиса CMap).
	const batch = 100
	for start := 0; start < len(ids); start += batch {
		end := start + batch
		if end > len(ids) {
			end = len(ids)
		}
		sb.WriteString(fmt.Sprintf("%d beginbfchar\n", end-start))
		for _, cid := range ids[start:end] {
			r := mapping[cid]
			if r <= 0xFFFF {
				sb.WriteString(fmt.Sprintf("<%04X> <%04X>\n", cid, r))
				continue
			}
			// Суррогатная пара (редкость — эмодзи в именах организаций).
			r -= 0x10000
			hi := uint16(0xD800 | (r >> 10))
			lo := uint16(0xDC00 | (r & 0x3FF))
			sb.WriteString(fmt.Sprintf("<%04X> <%04X%04X>\n", cid, hi, lo))
		}
		sb.WriteString("endbfchar\n")
	}
	sb.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	stream, err := core.MakeStream([]byte(sb.String()), core.NewRawEncoder())
	if err != nil {
		return nil, fmt.Errorf("fontutil: ToUnicode CMap: %w", err)
	}
	return stream, nil
}

// SubsetAll навешивает ToUnicode CMap на внедрённые шрифты. Сабсет шрифтов
// намеренно НЕ выполняется: RunesToCharcodeBytes у составного шрифта первым
// декодирует через toUnicode CMap (а не через TrueType-энкодер), поэтому руны
// не регистрируются в энкодере, и SubsetKeepRunes собирает ПУСТОЙ сабсет —
// внедрённые глифы исчезают, текст становится невидимым. Вместо сабсета
// внедряем полный TrueType: CID==GID исходного шрифта стабильны и в точности
// соответствуют гекс-литералам, уже записанным в потоки страниц.
func (m *FontManager) SubsetAll() error {
	for fam := range m.fonts {
		// ToUnicode CMap из записанных при кодировании пар CID→rune.
		st, err := m.buildToUnicode(fam)
		if err != nil {
			return err
		}
		if st == nil {
			continue
		}
		if d, ok := core.GetDict(m.objs[fam]); ok {
			d.Set("ToUnicode", st)
			// Перекрываем /W (и /DW) из ToPdfObject: unipdf строит его по
			// ключу Unicode-кода, а не по CID. Мы же рисуем текст CIDs==GID
			// (Identity-H), и Acrobat берёт ширины именно из /W — при расхождении
			// ключей глифы получают «дефолтную» ширину: заглавные слипаются,
			// строчные «ответственностью» наоборот слишком широко. Подменяем
			// массив ширин: /W[i] = истинный аванс глифа (тысячные доли em),
			// /DW — ширина по умолчанию для отсутствующих CID.
			// /W живёт в словаре CIDFontType2 (потомка Type0), а не в самом
			// Type0: правим именно потомка.
			if cid := cidFontDictOf(m.objs[fam]); cid != nil {
				if w := m.buildWidthArray(fam); w != nil {
					cid.Set("W", w)
					if dw := m.defaultWidth(fam); dw > 0 {
						cid.Set("DW", core.MakeFloat(dw))
					}
				}
			}
		}
	}
	return nil
}

// cidFontDictOf достаёт словарь CIDFontType2-потомка из объекта Type0: по
// /DescendantFonts[0] (у unipdf потомок — inline-словарь). Возвращает nil,
// если структура не распознана (не влияет на запись: остаётся /W unipdf).
func cidFontDictOf(obj core.PdfObject) *core.PdfObjectDictionary {
	d, ok := core.GetDict(obj)
	if !ok {
		return nil
	}
	arr, ok := core.GetArray(d.Get("DescendantFonts"))
	if !ok || arr.Len() == 0 {
		return nil
	}
	cd, ok := core.GetDict(core.TraceToDirectObject(arr.Get(0)))
	if !ok {
		return nil
	}
	if st, ok := cd.Get("Subtype").(*core.PdfObjectName); ok && string(*st) == "CIDFontType2" {
		return cd
	}
	return nil
}

// glyphAdvances возвращает карту CID→аванс глифа (тысячные доли em) для
// внедряемого шрифта семейства. Ключи — записанные при кодировании CIDs
// (CID == GID Identity-H); значения — истинные метрики шрифта GetRuneMetrics
// (тот же источник, что и NewWidth использует для геометрии страницы).
func (m *FontManager) glyphAdvances(fam string) map[uint16]float64 {
	f, ok := m.fonts[fam]
	if !ok {
		return nil
	}
	mapping := m.cmaps[fam]
	if len(mapping) == 0 {
		return nil
	}
	out := make(map[uint16]float64, len(mapping))
	for cid, r := range mapping {
		if cm, ok := f.GetRuneMetrics(r); ok {
			out[cid] = cm.Wx
		}
	}
	return out
}

// defaultWidth возвращает медианную ширину глифов семейства как /DW.
func (m *FontManager) defaultWidth(fam string) float64 {
	adv := m.glyphAdvances(fam)
	if len(adv) == 0 {
		return 0
	}
	vals := make([]int, 0, len(adv))
	for _, w := range adv {
		vals = append(vals, int(w))
	}
	sort.Ints(vals)
	return float64(vals[len(vals)/2])
}

// buildWidthArray собирает массив /W (PDF CID font): пары «диапазон CID → ширина»
// в приходимом формате «c1 c2 w» (c1==c2 — одиночный CID), где CID — записанный
// Identity CID, а w — истинный аванс глифа в тысячных долях em. Соседние CID с
// равной шириной сливаются в один диапазон для компактности.
func (m *FontManager) buildWidthArray(fam string) core.PdfObject {
	adv := m.glyphAdvances(fam)
	if len(adv) == 0 {
		return nil
	}
	cids := make([]int, 0, len(adv))
	for c := range adv {
		cids = append(cids, int(c))
	}
	sort.Ints(cids)

	arr := core.MakeArray()
	i := 0
	for i < len(cids) {
		start := cids[i]
		wid := int(adv[uint16(start)])
		// Расширяем диапазон, пока соседние CID имеют ту же ширину.
		end := start
		for i+1 < len(cids) && cids[i+1] == end+1 && int(adv[uint16(cids[i+1])]) == wid {
			end = cids[i+1]
			i++
		}
		arr.Append(core.MakeInteger(int64(start)))
		arr.Append(core.MakeInteger(int64(end)))
		arr.Append(core.MakeInteger(int64(wid)))
		i++
	}
	return arr
}

// PageFontContext — шрифтовый контекст одной страницы: оригинальные шрифты её
// ресурсов, внедрённые шрифты и имена ресурсов под них.
type PageFontContext struct {
	manager  *FontManager
	page     *model.PdfPage
	res      *model.PdfPageResources
	orig     map[string]*model.PdfFont // имя ресурса ("F1") → оригинальный шрифт
	seen     map[string]bool           // имена ресурсов, уже извлечённые в orig
	embedded map[string]string         // семейство → имя ресурса ("TF1")
	subsets  int                       // счётчик для уникальных имён ресурсов
}

// NewPageFontContext создаёт контекст для страницы. Для страниц, читаемых
// model.NewPdfReader, page.Resources уже наполнен (включая наследуемые ресурсы).
func NewPageFontContext(m *FontManager, page *model.PdfPage) *PageFontContext {
	return &PageFontContext{
		manager:  m,
		page:     page,
		res:      page.Resources,
		orig:     map[string]*model.PdfFont{},
		seen:     map[string]bool{},
		embedded: map[string]string{},
	}
}

// Provider возвращает CodecProvider страницы: ран с итоговым текстом, который
// активный шрифт рана не покрывает, переключается на внедрённый TrueType;
// покрытый текст остаётся в исходном шрифте (originalCodec: ширина 0, нативный
// алфавит шрифта — натуральная вёрстка без кернинговой компенсации).
func (pfc *PageFontContext) Provider() CodecProvider {
	return func(font, text string) RunCodec {
		if font == "" || text == "" {
			return nil
		}
		orig, ok := pfc.originalFont(font)
		if !ok || orig == nil {
			return nil
		}
		// Шрифт покрывает текст целиком — остаёмся в нём.
		if coveredBy(orig, text) {
			return &originalCodec{orig: orig}
		}
		fam, embed, err := pfc.manager.FontForLogical(logicalFor(baseFontName(orig)), text)
		if err != nil {
			slog.Debug("fontutil: внедрение шрифта недоступно", slog.String("err", err.Error()))
			return nil
		}
		key, err := pfc.register(embed, fam)
		if err != nil {
			slog.Debug("fontutil: регистрация шрифта в ресурсах страницы", slog.String("err", err.Error()))
			return nil
		}
		return &embeddedCodec{orig: orig, embed: embed, fam: fam, m: pfc.manager, key: "/" + key}
	}
}

// Decoder возвращает функцию трансляции логического текста рана в Unicode до
// поиска плейсхолдеров. Для композитных (CID) шрифтов декодирует байты-символы
// через ToUnicode-карту (hex-CID сабсеты MS Word, где CID ≠ GID); простые
// шрифты (WinAnsi-литералы) возвращаются без изменений. Если карта не дала
// результата, исходный текст сохраняется.
func (pfc *PageFontContext) Decoder() func(font, text string) string {
	return func(font, text string) string {
		orig, ok := pfc.originalFont(font)
		if !ok || orig == nil || !orig.IsCID() {
			return text
		}
		s, hits, miss := orig.CharcodeBytesToUnicode([]byte(text))
		if miss > 0 {
			slog.Debug("fontutil: декодирование CID шрифта с пропусками", slog.String("font", font), slog.Int("miss", miss))
		}
		if hits > 0 && s != "" {
			return s
		}
		return text
	}
}

// originalFont возвращает шрифт ресурса страницы по имени ("/F1" → "F1").
func (pfc *PageFontContext) originalFont(name string) (*model.PdfFont, bool) {
	name = strings.TrimPrefix(name, "/")
	if pfc.seen[name] {
		return pfc.orig[name], true
	}
	pfc.seen[name] = true
	if pfc.res == nil {
		return nil, true
	}
	obj, ok := pfc.res.GetFontByName(core.PdfObjectName(name))
	if !ok {
		return nil, true
	}
	f, err := model.NewPdfFontFromPdfObject(obj)
	if err != nil {
		slog.Debug("fontutil: шрифт ресурса не разобран", slog.String("font", name), slog.String("err", err.Error()))
		return nil, true
	}
	pfc.orig[name] = f
	return f, true
}

// register регистрирует внедряемый шрифт в ресурсах страницы под уникальным
// именем TF<seq> (одно на семейство на страницу) и возвращает это имя.
func (pfc *PageFontContext) register(f *model.PdfFont, fam string) (string, error) {
	if key, ok := pfc.embedded[fam]; ok {
		return key, nil
	}
	if pfc.res == nil {
		return "", fmt.Errorf("fontutil: у страницы нет ресурсов для внедрения шрифта")
	}
	pfc.subsets++
	key := fmt.Sprintf("TF%d", pfc.subsets)
	obj, err := pfc.manager.FontObject(fam)
	if err != nil {
		return "", err
	}
	if err := pfc.res.SetFontByName(core.PdfObjectName(key), obj); err != nil {
		return "", fmt.Errorf("fontutil: SetFontByName(%s): %w", key, err)
	}
	pfc.embedded[fam] = key
	return key, nil
}

// embeddedCodec — кодек рана, переключённого на внедрённый TrueType-шрифт.
// Текст кодируется двухбайтовыми CID (Identity-H, CID == GID) и печатается
// гекс-литералом. При кодировании ведётся карта CID→rune для ToUnicode CMap.
type embeddedCodec struct {
	orig  *model.PdfFont // шрифт, которым был нарисован плейсхолдер
	embed *model.PdfFont // внедряемый шрифт
	fam   string         // семейство внедряемого шрифта (ключ менеджера)
	m     *FontManager   // менеджер (для записи CID→rune)
	key   string         // имя ресурса нового шрифта (/TF1)
}

// Encode возвращает CID-байты текста во внедряемом шрифте и запоминает
// пары CID→rune (по ним SubsetAll построит ToUnicode CMap).
func (c *embeddedCodec) Encode(utf8 string) string {
	c.m.record(c.fam, c.embed, utf8)
	b, miss := c.embed.StringToCharcodeBytes(utf8)
	if miss > 0 {
		slog.Warn("fontutil: символы не вошли во внедряемый шрифт", slog.String("font", c.fam), slog.Int("miss", miss))
	}
	return string(b)
}

// Hex — true: содержимое литерала двухбайтовое, печатаем <...>.
func (c *embeddedCodec) Hex() bool { return true }

// Width — ширина текста в исходном шрифте (тысячные доли em).
func (c *embeddedCodec) Width(utf8 string) float64 {
	return runeWidth(c.orig, utf8)
}

// NewWidth — ширина текста во внедряемом шрифте (тысячные доли em).
func (c *embeddedCodec) NewWidth(utf8 string) float64 {
	return runeWidth(c.embed, utf8)
}

// FontName — имя ресурса внедряемого шрифта для подстановки в Tf.
func (c *embeddedCodec) FontName() string { return c.key }

// originalCodec — кодек рана, текст которого остаётся в исходном шрифте
// (покрыт им полностью). Encode кодирует текст нативным алфавитом шрифта
// (CID для Type0/Identity-H, байты для простых). Ширины нулевые: значение
// рисуется естественной шириной и не расширяет поле — вёрстка повторяет
// исходный документ, кернинговой компенсации не требуется. FontName пуст —
// оператор Tf не переименовывается.
type originalCodec struct {
	orig *model.PdfFont
}

// Encode кодирует текст алфавитом исходного шрифта (CID-байты для Type0).
func (c *originalCodec) Encode(utf8 string) string {
	b, miss := c.orig.StringToCharcodeBytes(utf8)
	if miss > 0 {
		slog.Warn("fontutil: символы не вошли в исходный шрифт", slog.String("font", c.orig.BaseFont()), slog.Int("miss", miss))
	}
	return string(b)
}

// Hex — true для составных шрифтов (Identity-H, двухбайтовые коды).
func (c *originalCodec) Hex() bool { return isComposite(c.orig) }

// Width и NewWidth нулевые — замены не влияют на ширину и позиции.
func (c *originalCodec) Width(string) float64    { return 0 }
func (c *originalCodec) NewWidth(string) float64 { return 0 }

// FontName пуст: шрифт рана не меняется.
func (c *originalCodec) FontName() string { return "" }

// runeWidth суммирует метрики символов текста (CharMetrics.Wx в тысячных em).
func runeWidth(f *model.PdfFont, text string) float64 {
	var sum float64
	for _, r := range text {
		if m, ok := f.GetRuneMetrics(r); ok {
			sum += m.Wx
		}
	}
	return sum
}
