package pdfpl

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	core "github.com/oliverpool/unipdf/v3/core"
	model "github.com/oliverpool/unipdf/v3/model"
	"github.com/unidoc/unitype"
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

// logicalFor извлекает логическое имя шрифта из BaseFont PDF-шрифта, срезая
// префикс сабсета формата "NNNNN+" (создаётся Word при субсеттинге).
func logicalFor(baseFont string) string {
	if i := strings.IndexByte(baseFont, '+'); i >= 0 {
		return baseFont[i+1:]
	}
	return baseFont
}

// fontCovers проверяет через unipdf, покрывает ли шрифт весь текст целиком:
// ни одна руна не должна остаться без глифа (miss == 0, у Identity CID==GID).
func fontCovers(f *model.PdfFont, text string) bool {
	_, miss := f.RunesToCharcodeBytes([]rune(text))
	return miss == 0
}

// boldishName — есть ли в имени файла намёк на жирное начертание. Используется
// только для приоритета порядка перебора фолбека «что есть» под Bold-имя.
func boldishName(path string) bool {
	n := fontNorm(filepath.Base(path))
	for _, tok := range []string{"black", "heavy", "bd"} {
		if strings.Contains(n, tok) {
			return true
		}
	}
	return false
}

// findFontCovering возвращает системные TrueType-файлы (каталоги системы) в
// порядке приоритета для подбора «до того, что есть»: никаких захардкоженных
// имён. Для жирного логического имени первыми идут буквально «bold»-файлы,
// затем прочие «жирные» (black/heavy/bd), и только потом остальные.
func findFontCovering(bold bool) []string {
	var first, fat, plain []string
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
			if !strings.HasSuffix(low, ".ttf") && !strings.HasSuffix(low, ".otf") {
				continue
			}
			p := filepath.Join(d, ent.Name())
			if !bold {
				plain = append(plain, p)
				continue
			}
			switch {
			case strings.Contains(fontNorm(ent.Name()), "bold"):
				first = append(first, p)
			case boldishName(p):
				fat = append(fat, p)
			default:
				plain = append(plain, p)
			}
		}
	}
	if !bold {
		return plain
	}
	return append(append(first, fat...), plain...)
}

// resolveEmbeddedFile подбирает системный TrueType-файл под внедряемый текст:
// сначала точное PostScript-имя логического имени BaseFont из PDF среди
// установленных шрифтов (findFontByName), затем — первый доступный шрифт,
// покрывающий текст (фолбек «до того, что есть»). Возвращает "" — подходящего
// шрифта нет, остаётся оригинальный шрифт документа.
func resolveEmbeddedFile(logical, text string) string {
	if p := findFontByName(logical); p != "" {
		return p
	}
	for _, p := range findFontCovering(strings.Contains(logical, "Bold")) {
		f, err := model.NewCompositePdfFontFromTTFFile(p)
		if err != nil {
			continue
		}
		if fontCovers(f, text) {
			return p
		}
	}
	return ""
}

// fontManager кэширует внедряемые TrueType-шрифты (один на логическое имя на
// файл). Для каждого шрифта ведётся карта CID→rune (CID == GID Identity-H), на
// основе которой после обработки страниц строится ToUnicode CMap — без него
// экстрактор текста не может восстановить Unicode из гекс-литералов.
type fontManager struct {
	fonts  map[string]*model.PdfFont  // логическое имя шрифта → загруженный шрифт
	objs   map[string]core.PdfObject  // логическое имя → PDF-объект шрифта в ресурсах
	cmaps  map[string]map[uint16]rune // логическое имя → CID→rune
	cached []*model.PdfFont           // все загруженные — для SubsetRegistered()
}

func newFontManager() *fontManager {
	return &fontManager{
		fonts: map[string]*model.PdfFont{},
		objs:  map[string]core.PdfObject{},
		cmaps: map[string]map[uint16]rune{},
	}
}

// fontForLogical загружает (с кэшем) системный TrueType под внедряемый текст:
// шрифт подбирается по логическому имени BaseFont (точное PS-имя) либо, если
// его нет — любой установленный шрифт, покрывающий текст (фолбек «что есть»).
// Кэш и ресурсное пространство имён используют логическое имя, а не файл:
// кодек привязан к исходному рану, а не к физическому файлу шрифта.
func (m *fontManager) fontForLogical(logical, text string) (string, *model.PdfFont, error) {
	if f, ok := m.fonts[logical]; ok {
		return logical, f, nil
	}
	ttf := resolveEmbeddedFile(logical, text)
	if ttf == "" {
		return "", nil, fmt.Errorf("pdfpl: системный шрифт %q не найден", logical)
	}
	f, err := model.NewCompositePdfFontFromTTFFile(ttf)
	if err != nil {
		return "", nil, fmt.Errorf("pdfpl: загрузка TrueType %s: %w", ttf, err)
	}
	m.fonts[logical] = f
	m.cached = append(m.cached, f)
	return logical, f, nil
}

// fontObject возвращает кэшированный PDF-объект шрифта логического имени: один
// на имя, чтобы ToUnicode-стрим (добавляемый в subsetAll) попал в тот же
// объект, что ссылки ресурсов всех страниц.
func (m *fontManager) fontObject(fam string) (core.PdfObject, error) {
	if o, ok := m.objs[fam]; ok {
		return o, nil
	}
	_, f, err := m.fontForLogical(fam, "")
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

// systemFontDirs — системные каталоги TrueType-шрифтов для текущей платформы:
// Windows (WINDIR\Fonts и пер-пользовательские %LOCALAPPDATA%\...\Fonts), а
// также стандартные каталоги Linux (Liberation/msttcorefonts) для
// кросс-платформенной сборки.
func systemFontDirs() []string {
	var dirs []string
	if wd := os.Getenv("WINDIR"); wd != "" {
		dirs = append(dirs, filepath.Join(wd, "Fonts"))
	}
	if sr := os.Getenv("SystemRoot"); sr != "" && sr != os.Getenv("WINDIR") {
		dirs = append(dirs, filepath.Join(sr, "Fonts"))
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		dirs = append(dirs, filepath.Join(local, "Microsoft", "Windows", "Fonts"))
	}
	dirs = append(dirs,
		"/usr/share/fonts/truetype/liberation",
		"/usr/share/fonts/truetype/msttcorefonts",
		"/usr/share/fonts/truetype/dejavu",
		"/usr/share/fonts",
	)
	return dirs
}

// fontNorm приводит имя шрифта (имя файла или PostScript-имя) к единому виду
// для сравнения: регистр в нижний, неалфавитно-цифровые символы вон. Так
// "XO_Thames_Bu" и "xothamesbold" становятся сравнимы.
func fontNorm(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// fontFamilyOf оставляет от PostScript-имени шрифта только семейство: всё до
// первого дефиса (стилевая часть отделена именно дефисом — "XOThames-Bold",
// "TimesNewRomanPS-BoldMT") плюс срезание хвостового "MT" из метрически
// совместимых Windows-имён. Так "XOThames-Bold" и "TimesNewRomanPSMT" сводятся
// к "xothames" и "timesnewromanps" соответственно.
func fontFamilyOf(name string) string {
	fam := name
	if i := strings.IndexByte(fam, '-'); i >= 0 {
		fam = fam[:i]
	}
	return strings.TrimSuffix(fontNorm(fam), "mt")
}

// fontBoldish — похоже ли имя шрифта на жирное начертание по своим токенам.
func fontBoldish(name string) bool {
	n := fontNorm(name)
	for _, tok := range []string{"bold", "extrabold", "semibold", "demibold", "black", "heavy", "bd"} {
		if strings.Contains(n, tok) {
			return true
		}
	}
	return false
}

// italicish — похоже ли имя шрифта на курсивное начертание по своим токенам.
func italicish(name string) bool {
	n := fontNorm(name)
	return strings.Contains(n, "italic") || strings.Contains(n, "oblique")
}

// findFontByName ищет в системных каталогах шрифтов файл с совпадающим
// семейством, весом и начертанием (курсив/прямое). Разбор unipdf выполняется
// только для файлов, чьё имя намекает на искомое семейство (полное имя либо
// первые 5 символов — "timesbd" отыщет "TimesNewRomanPS"), поэтому работаем
// быстро и кросс-платформенно — без завязки на Windows-реестр. Возвращает путь
// к файлу с точным совпадением стиля либо лучший неполный вариант, либо "".
func findFontByName(logical string) string {
	if logical == "" {
		return ""
	}
	lfam := fontFamilyOf(logical)
	lbold := fontBoldish(logical)
	lit := italicish(logical)
	hint := lfam
	if r := []rune(hint); len(r) > 5 {
		hint = string(r[:5])
	}
	best := ""
	bestScore := -1
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
			if !strings.HasSuffix(low, ".ttf") && !strings.HasSuffix(low, ".otf") {
				continue
			}
			fn := fontNorm(ent.Name())
			if !strings.Contains(fn, lfam) && !strings.Contains(fn, hint) {
				continue
			}
			p := filepath.Join(d, ent.Name())
			f, err := model.NewCompositePdfFontFromTTFFile(p)
			if err != nil {
				continue
			}
			if fontFamilyOf(f.BaseFont()) != lfam {
				continue
			}
			isBd := fontBoldish(f.BaseFont())
			isIt := italicish(f.BaseFont())
			score := 0
			if isIt == lit {
				score += 4
			}
			if isBd == lbold {
				score += 2
			}
			if lbold && isBd {
				score++
			}
			if !isIt {
				score++
			}
			if score == 8 {
				return p
			}
			if score > bestScore {
				bestScore, best = score, p
			}
		}
	}
	return best
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
// активный шрифт рана не покрывает, переключается на внедрённый TrueType;
// покрытый текст остаётся в исходном шрифте (originalCodec: ширина 0, нативный
// алфавит шрифта — натуральная вёрстка без кернинговой компенсации).
func (pfc *pageFontContext) provider() CodecProvider {
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
		fam, embed, err := pfc.manager.fontForLogical(logicalFor(baseFontName(orig)), text)
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

// coveredBy проверяет, что шрифт может отрисовать каждый символ текста.
// Критерий надёжности — не только возможность закодировать руны, но и наличие
// реальных глифов во внедрённой TrueType-программе шрифта (cmap → GID != 0):
// сабсет-шрифты Word (NNNNN+…) содержат глифы только для реально использованных
// в документе символов. Составные шрифты (Type0/Identity-H) покрыты, если все
// руны есть в cmap внедрённого сабсета (CID == GID, текст кодируется штатными
// CharcodeBytes); цифры в сабсете XOThames дают GID=0 → .notdef, и такой ран
// переключается на внедрённый шрифт. Простые шрифты проверяются по
// RunesToCharcodeBytes (misses == 0, без нулевых WinAnsi-байтов) и дополнительно
// по cmap, если программа шрифта внедрена.
func coveredBy(f *model.PdfFont, text string) bool {
	if f == nil || text == "" {
		return false
	}
	covered := fontCmap(f)

	if isComposite(f) {
		if covered == nil {
			return false
		}
		for _, r := range []rune(text) {
			if !covered[r] {
				return false
			}
		}
		return true
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

	if covered != nil {
		// Внедрённый TrueType-сабсет может вычистить глифы (цифры XOThames →
		// GID 0, WinAnsi-энкодер отвечает байтом): проверяем cmap по-рунно.
		for _, r := range []rune(text) {
			if !covered[r] {
				return false
			}
		}
		return true
	}
	// Без внедрённой программы (base-14) полагаемся на байтовый критерий, но
	// сабсет-имя без программы проверить нельзя — считаем его непокрытым.
	return !isSubsetFontName(baseFontName(f))
}

// subsetCmapCache кэширует множество рун с реальным глифом (GID != 0) по
// экземпляру шрифта; распарсено один раз на документ.
var subsetCmapCache sync.Map // *model.PdfFont → map[rune]bool (nil — программы нет)

// fontCmap возвращает руны, для которых во внедрённой TrueType-программе шрифта
// есть глиф, либо nil, если программы нет или она не парсится.
func fontCmap(f *model.PdfFont) map[rune]bool {
	if v, ok := subsetCmapCache.Load(f); ok {
		return v.(map[rune]bool)
	}
	cov := parseFontCmap(f)
	subsetCmapCache.Store(f, cov)
	return cov
}

// parseFontCmap извлекает FontFile2 (TTF) шрифта и строит руны с GID != 0.
func parseFontCmap(f *model.PdfFont) map[rune]bool {
	desc, err := f.GetFontDescriptor()
	if err != nil || desc == nil || desc.FontFile2 == nil {
		return nil
	}
	stream, ok := core.GetStream(desc.FontFile2)
	if !ok {
		return nil
	}
	data, err := core.DecodeStream(stream)
	if err != nil {
		return nil
	}
	sfnt, err := unitype.Parse(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	// Порядок поиска, как в unitype LookupRunes: (3,1), (1,0), (0,3).
	maps := []map[rune]unitype.GlyphIndex{
		sfnt.GetCmap(3, 1),
		sfnt.GetCmap(1, 0),
		sfnt.GetCmap(0, 3),
	}
	out := map[rune]bool{}
	for _, cm := range maps {
		if cm == nil {
			continue
		}
		for r, g := range cm {
			if g != 0 {
				out[r] = true
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
	b, _ := c.orig.StringToCharcodeBytes(utf8)
	return string(b)
}

// Hex — true для составных шрифтов (Identity-H, двухбайтовые коды).
func (c *originalCodec) Hex() bool { return isComposite(c.orig) }

// Width и NewWidth нулевые — замены не влияют на ширину и позиции.
func (c *originalCodec) Width(string) float64    { return 0 }
func (c *originalCodec) NewWidth(string) float64 { return 0 }

// FontName пуст: шрифт рана не меняется.
func (c *originalCodec) FontName() string { return "" }

// Экспортированные диагностические обёртки для внешних инструментов
// (демо, пробы, интеграционное тестирование).

// FontCmap возвращает руны с реальным глифом (GID != 0) во внедрённой
// TrueType-программе шрифта (nil — программы нет или она не парсится).
func FontCmap(f *model.PdfFont) map[rune]bool { return fontCmap(f) }

// CoveredBy — покрывает ли шрифт текст (см. coveredBy).
func CoveredBy(f *model.PdfFont, text string) bool { return coveredBy(f, text) }

// BaseFontName возвращает /BaseFont шрифта.
func BaseFontName(f *model.PdfFont) string { return baseFontName(f) }

// IsCompositeFont — составной ли шрифт (Subtype == Type0).
func IsCompositeFont(f *model.PdfFont) bool { return isComposite(f) }

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
