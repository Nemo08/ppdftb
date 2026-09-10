package pdfpl

import (
	"regexp"
	"strings"
)

// field — вхождение плейсхолдера {{.Key}} или {{Key}} в контентном потоке
// с абсолютными байтовыми границами и шрифтовым контекстом ран-блока.
type field struct {
	key       string  // имя поля без скобок (с точкой оставлен как в шаблоне)
	begin     int     // абсолютное смещение начала '{{' в потоке
	end       int     // абсолютное смещение конца (за '}}')
	font      string  // активный шрифт (/F1)
	size      float64 // размер шрифта из Tf
	placeText string  // отображаемый текст плейсхолдера (без скобок)
	run       int     // индекс рана в списке runs
}

// placeholderRe ловит {{.Key}}/{{Key}} в литеральных строках.
var placeholderRe = regexp.MustCompile(`\{\{\s*\.?([^{}\r\n]+?)\s*\}\}`)

// skipRangeBlock находит закрывающий "{{end}}" в text, начиная с start.
// Возвращает индекс сразу после "{{end}}" и true, если блок закрыт в этом
// тексте; false — блок продолжается в следующем (ран/конкатенация сплошные).
// Единая точка поиска конца range-блока для findFields и buildCrossRunFields.
func skipRangeBlock(text string, start int) (int, bool) {
	if ei := strings.Index(text[start:], "{{end}}"); ei >= 0 {
		return start + ei + len("{{end}}"), true
	}
	return 0, false
}

// findFields находит плейсхолдеры внутри операндов ранов, пропуская
// диапазоны {{range ...}}...{{end}} (range-блоки игнорируются целиком).
//
// Регулярное выражение применяется к ЛОГИЧЕСКОМУ тексту рана (r.text), а не к
// сырым байтам операнда: в TJ-массивах (типично для MS Word) фрагменты
// {{.ESYear}} разбиты на отдельные литералы [({{.)4(ESY)...], и открывающие/
// закрывающие скобки физически не соседствуют в байтах операнда. Логический
// текст собирается в textRuns (extractShowText) конкатенацией литералов.
func findFields(stream string, runs []streamRun) []field {
	var fields []field
	inRange := false
	for i, r := range runs {
		if inRange {
			if _, ok := skipRangeBlock(r.text, 0); ok {
				inRange = false
			}
			continue
		}
		if ri := strings.Index(r.text, "{{range"); ri >= 0 {
			collectFields(r.text[:ri], r, i, 0, &fields)
			if after, ok := skipRangeBlock(r.text, ri); ok {
				collectFields(r.text[after:], r, i, after, &fields)
			} else {
				inRange = true
			}
			continue
		}
		collectFields(r.text, r, i, 0, &fields)
	}
	return fields
}

// collectFields достаёт поля из slice (подстроки логического текста рана)
// и дописывает их в список. base — смещение начала slice в r.text.
// begin/end высчитываются относительно начала операнда (r.start) в байтах
// потока и ПРИБЛИЖЁННЫЕ: r.text — раскодированный контент литералов, а
// r.start указывает на разделитель (`(` для Tj, `[` для TJ-массива).
// Для замен (buildStream) используются только key и run, поэтому смещения
// здесь носят диагностический характер и не влияют на результат сборки.
func collectFields(slice string, r streamRun, runIdx int, base int, fields *[]field) {
	// Для одиночного литерала (xx) Tj контент начинается сразу за '(' —
	// точное смещение = r.start+1; для TJ-массивов с числами кернинга —
	// только эвристика r.start.
	shift := 0
	if r.opType == "Tj" {
		shift = 1
	}
	for _, m := range placeholderRe.FindAllStringSubmatchIndex(slice, -1) {
		bodyStart, bodyEnd := m[2], m[3]
		*fields = append(*fields, field{
			key:       strings.TrimSpace(slice[bodyStart:bodyEnd]),
			begin:     r.start + shift + base + m[0],
			end:       r.start + shift + base + m[1],
			font:      r.font,
			size:      r.size,
			placeText: slice[bodyStart:bodyEnd],
			run:       runIdx,
		})
	}
}
