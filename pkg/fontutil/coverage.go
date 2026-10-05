package fontutil

import (
	"bytes"
	"strings"
	"sync"

	core "github.com/oliverpool/unipdf/v3/core"
	model "github.com/oliverpool/unipdf/v3/model"
	"github.com/unidoc/unitype"
)

// fontCovers проверяет через unipdf, покрывает ли шрифт весь текст целиком:
// ни одна руна не должна остаться без глифа (miss == 0, у Identity CID==GID).
func fontCovers(f *model.PdfFont, text string) bool {
	_, miss := f.RunesToCharcodeBytes([]rune(text))
	return miss == 0
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
