package fontutil

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	model "github.com/oliverpool/unipdf/v3/model"
)

// logicalFor извлекает логическое имя шрифта из BaseFont PDF-шрифта, срезая
// префикс сабсета формата "NNNNN+" (создаётся Word при субсеттинге).
func logicalFor(baseFont string) string {
	if i := strings.IndexByte(baseFont, '+'); i >= 0 {
		return baseFont[i+1:]
	}
	return baseFont
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

// scoreFontStyle — балл соответствия начертания кандидата целевому запросу
// (единый для resolveEmbeddedFile и findFontByName, чтобы полный и фолбековый
// подборы ранжировали кандидатов одинаково):
//
//	+4  совпадение начертания (курсив/прямое)
//	+2  совпадение веса (жирное/обычное)
//	+1  жирный запрос к жирному кандидату — подчёркнутое предпочтение
//	+1  прямой (не курсивный) кандидат — прямые файлы предпочтительнее курсива
//
// Чем выше — тем кандидат предпочтительнее.
func scoreFontStyle(candBold, candItalic, targetBold, targetItalic bool) int {
	s := 0
	if candItalic == targetItalic {
		s += 4
	}
	if candBold == targetBold {
		s += 2
	}
	if targetBold && candBold {
		s++
	}
	if !candItalic {
		s++
	}
	return s
}

// findFontByName ищет в системных каталогах шрифтов файл с совпадающим
// семейством, весом и начертанием (курсив/прямое). Разбор unipdf выполняется
// только для файлов, чьё имя намекает на искомое семейство (полное имя либо
// первые 5 символов — "timesbd" отыщет "TimesNewRomanPS"), поэтому работаем
// быстро и кросс-платформенно — без завязки на Windows-реестр. Для
// cloud-каталогов (Aptos) семья задана ИМЕНЕМ ПОДКАТАЛОГА, а файлы начертаний
// внутри числовые — такие подкаталоги обходим целиком. Возвращает путь к файлу
// с точным совпадением стиля либо лучший неполный вариант, либо "".
//
// Разбор кандидатов идёт через loadFontFile (кэш по пути): вызов повторяется
// при каждой новой строке нумерации, а файлов-кандидатов у Arial девять
// (arial*, ARIALN*, ARIALUNI ≈ 8 МБ) — без кэша каждый промах перечитывал бы
// их заново.
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
		// Разбор unipdf делаем только для файлов, намекающих на семейство
		// (фильтр fontCandidates), поэтому работаем быстро и кросс-платформенно.
		for _, p := range fontCandidates(d, lfam, hint) {
			f, err := loadFontFile(p)
			if err != nil {
				slog.Debug("fontutil: не удалось разобрать TTF", slog.String("file", p), slog.String("err", err.Error()))
				continue
			}
			if fontFamilyOf(f.BaseFont()) != lfam {
				continue
			}
			score := scoreFontStyle(fontBoldish(f.BaseFont()), italicish(f.BaseFont()), lbold, lit)
			if score > bestScore {
				bestScore, best = score, p
			}
		}
	}
	if best == "" {
		slog.Debug("fontutil: системный шрифт под логическое имя не найден", slog.String("logical", logical), slog.String("family", lfam))
	}
	return best
}

// findFontCovering возвращает системные TrueType-файлы (каталоги системы) в
// порядке приоритета для подбора «до того, что есть»: никаких захардкоженных
// имён. Для жирного логического имени первыми идут буквально «bold»-файлы,
// затем прочие «жирные» (extrabold/semibold/demibold/black/heavy/bd — те же
// токены fontBoldish), и только потом остальные.
func findFontCovering(bold bool) []string {
	var first, fat, plain []string
	for _, d := range systemFontDirs() {
		for _, ref := range fontFilesInDir(d) {
			if !bold {
				plain = append(plain, ref.path)
				continue
			}
			switch {
			case strings.Contains(fontNorm(ref.name), "bold"):
				first = append(first, ref.path)
			case fontBoldish(filepath.Base(ref.path)):
				fat = append(fat, ref.path)
			default:
				plain = append(plain, ref.path)
			}
		}
	}
	if !bold {
		return plain
	}
	return append(append(first, fat...), plain...)
}

func resolveEmbeddedFile(logical, text string) string {
	if p := findFontByName(logical); p != "" {
		return p
	}
	lbold := fontBoldish(logical)
	lit := italicish(logical)
	best := ""
	bestScore := -1
	for _, p := range findFontCovering(lbold) {
		f, err := loadFontFile(p)
		if err != nil {
			slog.Debug("fontutil: не удалось разобрать TTF", slog.String("file", p), slog.String("err", err.Error()))
			continue
		}
		if !fontCovers(f, text) {
			continue
		}
		cb, ci := fontBoldish(f.BaseFont()), italicish(f.BaseFont())
		if cb == lbold && ci == lit {
			// Точное совпадение веса и начертания — берём первое решение.
			return p
		}
		if s := scoreFontStyle(cb, ci, lbold, lit); s > bestScore {
			best, bestScore = p, s
		}
	}
	return best
}

// resolveKey — ключ мемоизации Resolve: семейство + отсортированный набор рун.
// Один и тот же набор символов (цифры нумерации, «Приложение <буква>») даёт
// один ключ, поэтому системные каталоги сканируются один раз на документ.
func resolveKey(family, text string) string {
	seen := map[rune]bool{}
	rs := make([]rune, 0, len(text))
	for _, r := range text {
		if !seen[r] {
			seen[r] = true
			rs = append(rs, r)
		}
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i] < rs[j] })
	return family + "\x00" + string(rs)
}

// resolveEntry — результат Resolve: загруженный шрифт и путь к файлу-источнику
// (для диагностики в предупреждениях).
type resolveEntry struct {
	font *model.PdfFont
	file string
}

// resolveCache мемоизирует Resolve по набору рун: повторные вызовы на каждой
// странице не перечитывают системные каталоги и не перечитывают TTF с диска.
var resolveCache sync.Map // string → *resolveEntry

// familyCache мемоизирует Resolve по самому семейству: номера страниц дают
// каждый раз новый набор рун («12», «13», «Приложение А. 47»), поэтому по
// resolveCache они почти всегда промахиваются. Хранится шрифт, точно
// подобранный под семейство; при повторном вызове достаточно дешёво проверить
// fontCovers, чем сканировать системные каталоги и разбирать кандидатов.
//
// Нестрогое совпадение (exact=false, семейство не найдено) сюда не кладётся:
// иначе фолбэк закрепился бы за семейством и перекрывал бы реальный шрифт
// семейства, когда тот покрывает следующий текст (частый случай —
// Word-сабсет из цифр против текста с кириллицей).
var familyCache sync.Map // string → *resolveEntry

// fontFileCache кэширует разобранные TTF по пути. NewCompositePdfFontFromTTFFile
// сжимает файл целиком, поэтому повторная загрузка одного и того же системного
// шрифта (разные наборы рун — разные цифры номера страницы) дорога. Файл на
// диске не меняется в течение прогона, так что кэш безопасен.
//
// Используется только Resolve: FontManager держит собственные шрифты по
// семействам и полагается на отдельные PDF-объекты на семейство.
var fontFileCache sync.Map // string → *model.PdfFont

// loadFontFile загружает TrueType с кэшем по пути.
func loadFontFile(path string) (*model.PdfFont, error) {
	if v, ok := fontFileCache.Load(path); ok {
		return v.(*model.PdfFont), nil
	}
	f, err := model.NewCompositePdfFontFromTTFFile(path)
	if err != nil {
		return nil, err
	}
	fontFileCache.Store(path, f)
	return f, nil
}

// Resolve ищет в системных каталогах шрифт для отрисовки text и загружает его.
//
// family — логическое имя шрифта (PostScript-имя либо имя семейства, например
// "Arial"). Приоритет подбора:
//
//  1. файл с точным совпадением семейства и начертания, покрывающий text;
//  2. любой установленный шрифт, покрывающий text, с наибольшим баллом
//     начертания.
//
// Второй случай — не ошибка, а предупреждение в лог: текст отрисуется другим
// шрифтом. Ошибка возвращается только когда не нашлось ни одного подходящего
// файла либо ни один не удалось загрузить.
//
// Кэш двухуровневый: сначала точный hit по набору рун (resolveCache), затем
// по семейству (familyCache) с проверкой покрытия текста — на нумерации второй
// уровень срабатывает почти всегда, первый почти никогда.
func Resolve(family, text string) (*model.PdfFont, string, error) {
	if family == "" {
		return nil, "", fmt.Errorf("fontutil: пустое имя шрифта")
	}
	key := resolveKey(family, text)
	if v, ok := resolveCache.Load(key); ok {
		e := v.(*resolveEntry)
		return e.font, e.file, nil
	}
	if v, ok := familyCache.Load(family); ok {
		e := v.(*resolveEntry)
		if fontCovers(e.font, text) {
			resolveCache.Store(key, e)
			return e.font, e.file, nil
		}
	}
	font, file, exact, err := resolveUncached(family, text)
	if err != nil {
		return nil, "", err
	}
	if !exact {
		slog.Warn("fontutil: запрошенный шрифт недоступен или не покрывает текст, подставлен другой",
			slog.String("requested", family), slog.String("used", font.BaseFont()), slog.String("file", file))
	}
	e := &resolveEntry{font: font, file: file}
	resolveCache.Store(key, e)
	if exact {
		familyCache.Store(family, e)
	}
	return font, file, nil
}

// resolveUncached выполняет сам подбор без мемоизации. exact=false означает, что
// пришлось отступить от запрошенного семейства.
func resolveUncached(family, text string) (font *model.PdfFont, file string, exact bool, err error) {
	// 1) Точное совпадение семейства и начертания — но только если оно реально
	// покрывает текст: сабсет-шрифт может не содержать нужных глифов.
	if p := findFontByName(family); p != "" {
		if f, err := loadFontFile(p); err == nil {
			if fontCovers(f, text) {
				return f, p, true, nil
			}
			slog.Warn("fontutil: шрифт семейства не покрывает текст, ищем другой", slog.String("requested", family), slog.String("file", p))
		} else {
			slog.Debug("fontutil: не удалось загрузить TTF", slog.String("file", p), slog.String("err", err.Error()))
		}
	}

	// 2) Любой установленный шрифт, покрывающий текст, с наибольшим баллом
	// начертания. Ошибки загрузки и непокрытые кандидаты молча пропускаются —
	// их может быть много, а выбрать нужно один лучший.
	bold := fontBoldish(family)
	italic := italicish(family)
	var best *model.PdfFont
	bestFile := ""
	bestScore := -1
	for _, p := range findFontCovering(bold) {
		f, e := loadFontFile(p)
		if e != nil {
			slog.Debug("fontutil: не удалось разобрать TTF", slog.String("file", p), slog.String("err", e.Error()))
			continue
		}
		if !fontCovers(f, text) {
			continue
		}
		if s := scoreFontStyle(fontBoldish(f.BaseFont()), italicish(f.BaseFont()), bold, italic); s > bestScore {
			best, bestFile, bestScore = f, p, s
		}
	}
	if best == nil {
		return nil, "", false, fmt.Errorf("fontutil: не найден системный шрифт, покрывающий текст (запрошен %q)", family)
	}
	return best, bestFile, false, nil
}
