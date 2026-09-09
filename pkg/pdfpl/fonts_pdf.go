package pdfpl

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	core "github.com/oliverpool/unipdf/v3/core"
	model "github.com/oliverpool/unipdf/v3/model"
)

// Внедрение TrueType-шрифтов.
//
// Плейсхолдер-раны в целевых PDF (MS Word) сидят в простых ASCII-шрифтах
// (TrueType simple, WinAnsi), которые физически не содержат кириллических
// глифов, либо в CID-сабсетах (XOThames, Identity-H), чьи CID — номера глифов
// мини-сабсета, а не Unicode. Вставить произвольное русское значение такими
// «байтами» невозможно. Поэтому ран, значение которого не покрывается
// активным шрифтом, переключается на внедрённый TrueType-шрифт (Times/Arial
// с системного диска): переписывается оператор Tf (/F1 → /TF1), текст вставляется
// гекс-литералом <...> двухбайтовых CID, ширина берётся из реальных метрик.

// fontFamily — условное семейство внедряемого шрифта. Выбирается по BaseFont
// исходного шрифта рана и разрешается в системный TrueType-файл.
type fontFamily int

const (
	familyTimes fontFamily = iota
	familyTimesBold
	familyArial
	familyArialBold
)

// familyLabel — человекочитаемое имя семейства (для лога/ошибок).
func familyLabel(f fontFamily) string {
	switch f {
	case familyTimesBold:
		return "Times Bold"
	case familyArial:
		return "Arial"
	case familyArialBold:
		return "Arial Bold"
	default:
		return "Times"
	}
}

// familyFor подбирает семейство внедряемого шрифта по BaseFont исходного.
// Типичные PDF-шрифты (Times*, Helvetica) не глифуют кириллицу — подставляем
// ближайшее системное семейство. Болд-имена (в т.ч. сабсеты вида
// 'BCDGEE+XOThames-Bold') всегда резолвятся в жирный файл.
func familyFor(baseFont string) string {
	if strings.Contains(baseFont, "Bold") {
		if strings.Contains(baseFont, "Times") || strings.Contains(baseFont, "Serif") ||
			strings.Contains(baseFont, "Georgia") {
			return "timesbd.ttf"
		}
		if strings.Contains(baseFont, "Arial") || strings.Contains(baseFont, "Helvetica") ||
			strings.Contains(baseFont, "Sans") || strings.Contains(baseFont, "Verdana") ||
			strings.Contains(baseFont, "Calibri") || strings.Contains(baseFont, "Tahoma") {
			return "arialbd.ttf"
		}
		return "timesbd.ttf" // XOThames-* и прочие serif-bold по умолчанию
	}
	if strings.Contains(baseFont, "Arial") || strings.Contains(baseFont, "Helvetica") ||
		strings.Contains(baseFont, "Sans") || strings.Contains(baseFont, "Verdana") ||
		strings.Contains(baseFont, "Calibri") || strings.Contains(baseFont, "Tahoma") {
		return "arial.ttf"
	}
	if strings.Contains(baseFont, "Times") || strings.Contains(baseFont, "Serif") || strings.Contains(baseFont, "Georgia") {
		return "times.ttf"
	}
	return "times.ttf" // фолбэк: Times ближе всего к типовым PDF-шрифтам
}

// fontManager кэширует внедряемые TrueType-шрифты (один на семейство на файл).
// Для каждого шрифта ведётся карта CID→rune (CID == GID Identity-H), на основе
// которой после обработки страниц строится ToUnicode CMap — без него экстрактор
// текста не может восстановить Unicode из гекс-литералов.
type fontManager struct {
	fonts  map[string]*model.PdfFont  // имя семейства (файл) → загруженный шрифт
	objs   map[string]core.PdfObject  // имя семейства → PDF-объект шрифта в ресурсах
	cmaps  map[string]map[uint16]rune // имя семейства → CID→rune
	cached []*model.PdfFont           // все загруженные — для SubsetRegistered()
}

func newFontManager() *fontManager {
	return &fontManager{
		fonts: map[string]*model.PdfFont{},
		objs:  map[string]core.PdfObject{},
		cmaps: map[string]map[uint16]rune{},
	}
}

// fontFor загружает (с кэшем) TrueType-шрифт семейства с системного диска.
func (m *fontManager) fontFor(fam string) (*model.PdfFont, error) {
	if f, ok := m.fonts[fam]; ok {
		return f, nil
	}
	ttf := findSystemFont(fam)
	if ttf == "" {
		return nil, fmt.Errorf("pdfpl: системный TrueType %s не найден", fam)
	}
	f, err := model.NewCompositePdfFontFromTTFFile(ttf)
	if err != nil {
		return nil, fmt.Errorf("pdfpl: загрузка TrueType %s: %w", ttf, err)
	}
	m.fonts[fam] = f
	m.cached = append(m.cached, f)
	return f, nil
}

// fontObject возвращает кэшированный PDF-объект шрифта семейства: один на
// семейство, чтобы ToUnicode-стрим (добавляемый в subsetAll) попал в тот же
// объект, что ссылки ресурсов всех страниц.
func (m *fontManager) fontObject(fam string) (core.PdfObject, error) {
	if o, ok := m.objs[fam]; ok {
		return o, nil
	}
	f, err := m.fontFor(fam)
	if err != nil {
		return nil, err
	}
	o := f.ToPdfObject()
	m.objs[fam] = o
	return o, nil
}

// record фиксирует соответствие CID→rune по фактическим байтам кодирования
// (один символ кодируется отдельно — код стабилен и совпадает с GID).
func (m *fontManager) record(fam string, f *model.PdfFont, utf8 string) {
	mapping, ok := m.cmaps[fam]
	if !ok {
		mapping = map[uint16]rune{}
	}
	for _, r := range utf8 {
		b, _ := f.StringToCharcodeBytes(string(r))
		if len(b) == 2 {
			mapping[uint16(b[0])<<8|uint16(b[1])] = r
		} else if len(b) == 1 {
			mapping[uint16(b[0])] = r
		}
	}
	m.cmaps[fam] = mapping
}

// buildToUnicode строит поток CMap (CID→UTF-16BE) для семейства и подвешивает
// его ключом ToUnicode в PDF-объект внедряемого шрифта. Возвращает nil, если
// для семейства не было записано ни одного символа.
func (m *fontManager) buildToUnicode(fam string) (core.PdfObject, error) {
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
		return nil, fmt.Errorf("pdfpl: ToUnicode CMap: %w", err)
	}
	return stream, nil
}

// subsetAll навешивает ToUnicode CMap на внедрённые шрифты. Сабсет шрифтов
// намеренно НЕ выполняется: RunesToCharcodeBytes у составного шрифта первым
// декодирует через toUnicode CMap (а не через TrueType-энкодер), поэтому руны
// не регистрируются в энкодере, и SubsetKeepRunes собирает ПУСТОЙ сабсет —
// внедрённые глифы исчезают, текст становится невидимым. Вместо сабсета
// внедряем полный TrueType: CID==GID исходного шрифта стабильны и в точности
// соответствуют гекс-литералам, уже записанным в потоки страниц.
func (m *fontManager) subsetAll() error {
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
		}
	}
	return nil
}

// findSystemFont ищет TrueType-файл в типовых системных каталогах шрифтов:
// Windows (WINDIR\Fonts), а также стандартные каталоги Linux (Liberation,
// msttcorefonts) для кросс-платформенной сборки.
func findSystemFont(fam string) string {
	var dirs []string
	if wd := os.Getenv("WINDIR"); wd != "" {
		dirs = append(dirs, filepath.Join(wd, "Fonts"))
	}
	if sr := os.Getenv("SystemRoot"); sr != "" && sr != os.Getenv("WINDIR") {
		dirs = append(dirs, filepath.Join(sr, "Fonts"))
	}
	dirs = append(dirs,
		"/usr/share/fonts/truetype/liberation",
		"/usr/share/fonts/truetype/msttcorefonts",
		"/usr/share/fonts/truetype/dejavu",
		"/usr/share/fonts",
	)
	// В Linux вместо Times/Arial доступны метрические аналоги Liberation.
	var names []string
	switch fam {
	case "timesbd.ttf":
		names = []string{fam, "LiberationSerif-Bold.ttf", "Liberation Serif Bold.ttf"}
	case "arial.ttf":
		names = []string{fam, "LiberationSans-Regular.ttf", "Liberation Sans.ttf"}
	case "arialbd.ttf":
		names = []string{fam, "LiberationSans-Bold.ttf", "Liberation Sans Bold.ttf"}
	default:
		names = []string{fam, "LiberationSerif-Regular.ttf", "Liberation Serif.ttf"}
	}
	for _, d := range dirs {
		for _, n := range names {
			p := filepath.Join(d, n)
			if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
				return p
			}
		}
	}
	return ""
}

// pageFontContext — шрифтовый контекст страницы: ссылки на оригинальные шрифты
// ресурсов (для проверки покрытия глифов) и внедряемые шрифты, зарегистрированные
// в ресурсах страницы.
type pageFontContext struct {
	manager  *fontManager
	page     *model.PdfPage
	res      *model.PdfPageResources
	orig     map[string]*model.PdfFont // имя ресурса ("F1") → оригинальный шрифт
	seen     map[string]bool           // имена ресурсов, уже извлечённые в orig
	embedded map[string]string         // семейство → имя ресурса ("TF1")
	subsets  int                       // счётчик для уникальных имён ресурсов
}

// newPageFontContext создаёт контекст для страницы. Для страниц, читаемых
// model.NewPdfReader, page.Resources уже наполнен (включая наследуемые ресурсы).
func newPageFontContext(m *fontManager, page *model.PdfPage) *pageFontContext {
	return &pageFontContext{
		manager:  m,
		page:     page,
		res:      page.Resources,
		orig:     map[string]*model.PdfFont{},
		seen:     map[string]bool{},
		embedded: map[string]string{},
	}
}

// provider возвращает CodecProvider страницы: ран с итоговым текстом, который
// активный шрифт рана не покрывает, переключается на внедрённый TrueType.
func (pfc *pageFontContext) provider() CodecProvider {
	return func(font, text string) RunCodec {
		if font == "" || text == "" {
			return nil
		}
		orig, ok := pfc.originalFont(font)
		if !ok || orig == nil {
			return nil
		}
		// Шрифт покрывает текст целиком — переключение не нужно.
		if coveredBy(orig, text) {
			return nil
		}
		fam := familyFor(baseFontName(orig))
		embed, err := pfc.manager.fontFor(fam)
		if err != nil {
			slog.Debug("pdfpl: внедрение шрифта недоступно", slog.String("err", err.Error()))
			return nil
		}
		key, err := pfc.register(embed, fam)
		if err != nil {
			slog.Debug("pdfpl: регистрация шрифта в ресурсах страницы", slog.String("err", err.Error()))
			return nil
		}
		return &embeddedCodec{orig: orig, embed: embed, fam: fam, m: pfc.manager, key: "/" + key}
	}
}

// originalFont возвращает шрифт ресурса страницы по имени ("/F1" → "F1").
func (pfc *pageFontContext) originalFont(name string) (*model.PdfFont, bool) {
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
		slog.Debug("pdfpl: шрифт ресурса не разобран", slog.String("font", name), slog.String("err", err.Error()))
		return nil, true
	}
	pfc.orig[name] = f
	return f, true
}

// register регистрирует внедряемый шрифт в ресурсах страницы под уникальным
// именем TF<seq> (одно на семейство на страницу) и возвращает это имя.
func (pfc *pageFontContext) register(f *model.PdfFont, fam string) (string, error) {
	if key, ok := pfc.embedded[fam]; ok {
		return key, nil
	}
	if pfc.res == nil {
		return "", fmt.Errorf("pdfpl: у страницы нет ресурсов для внедрения шрифта")
	}
	pfc.subsets++
	key := fmt.Sprintf("TF%d", pfc.subsets)
	obj, err := pfc.manager.fontObject(fam)
	if err != nil {
		return "", err
	}
	if err := pfc.res.SetFontByName(core.PdfObjectName(key), obj); err != nil {
		return "", fmt.Errorf("pdfpl: SetFontByName(%s): %w", key, err)
	}
	pfc.embedded[fam] = key
	return key, nil
}

// baseFontName возвращает /BaseFont шрифта из его PDF-словаря.
func baseFontName(f *model.PdfFont) string {
	d, ok := core.GetDict(f.ToPdfObject())
	if !ok {
		return ""
	}
	if n, ok := d.Get("BaseFont").(*core.PdfObjectName); ok {
		return string(*n)
	}
	return ""
}

// coveredBy проверяет, что шрифт может отрисовать каждый руна текста.
// Надёжный критерий — RunesToCharcodeBytes (число непокрытых рун == 0),
// а НЕ GetRuneMetrics: тот возвращает missingWidth с ok=true и не отличает
// отсутствующий глиф от реального (метрики словаря). Составные шрифты
// (Type0/Identity-H) всегда считаются непокрытыми: их CID — номера глифов
// сабсета, а не Unicode, произвольный текст в них не вставишь.
// Сабсет-шрифты (имя вида 'ABCDEF+XOThames', внедрённый TrueType) тоже
// непокрыты: WinAnsi-энкодер отвечает байтом даже для символа без глифа в
// сабсете, отчего значение превращается в .notdef-квадратики с нулевыми
// ширинами (буквы слипаются) — такой ран переключаем на внедрённый шрифт.
func coveredBy(f *model.PdfFont, text string) bool {
	if f == nil || text == "" {
		return false
	}
	if isComposite(f) {
		return false
	}
	if isSubsetFontName(baseFontName(f)) {
		return false
	}
	b, misses := f.RunesToCharcodeBytes([]rune(text))
	if misses != 0 {
		return false
	}
	// WinAnsi-энкодер отображает неизвестные руны в байт 0x00 (не глиф):
	// такие байты считаем непокрытыми, иначе кириллица «уедет» в notdef.
	for _, c := range b {
		if c == 0 {
			return false
		}
	}
	return true
}

// isSubsetFontName определяет сабсет-шрифт по /BaseFont: по PDF 32000 (9.6.4)
// внедрённые сабсеты именуются как 'NNNNN+' префикс перед именем шрифта
// (например 'BCDEEE+XOThames-Regular' у Word).
func isSubsetFontName(base string) bool {
	return strings.Contains(base, "+")
}

// isComposite определяет составной шрифт (Type0) по /Subtype словаря.
func isComposite(f *model.PdfFont) bool {
	d, ok := core.GetDict(f.ToPdfObject())
	if !ok {
		return false
	}
	if s, ok := d.Get("Subtype").(*core.PdfObjectName); ok {
		return string(*s) == "Type0"
	}
	return false
}

// embeddedCodec — кодек рана, переключённого на внедрённый TrueType-шрифт.
// Текст кодируется двухбайтовыми CID (Identity-H, CID == GID) и печатается
// гекс-литералом. При кодировании ведётся карта CID→rune для ToUnicode CMap.
type embeddedCodec struct {
	orig  *model.PdfFont // шрифт, которым был нарисован плейсхолдер
	embed *model.PdfFont // внедряемый шрифт
	fam   string         // семейство внедряемого шрифта (ключ менеджера)
	m     *fontManager   // менеджер (для записи CID→rune)
	key   string         // имя ресурса нового шрифта (/TF1)
}

// Encode возвращает CID-байты текста во внедряемом шрифте и запоминает
// пары CID→rune (по ним subsetAll построит ToUnicode CMap).
func (c *embeddedCodec) Encode(utf8 string) string {
	c.m.record(c.fam, c.embed, utf8)
	b, _ := c.embed.StringToCharcodeBytes(utf8)
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
