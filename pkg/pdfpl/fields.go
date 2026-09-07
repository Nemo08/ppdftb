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

// findFields находит плейсхолдеры внутри операндов ранов, пропуская
// диапазоны {{range ...}}...{{end}} (range-блоки игнорируются целиком).
func findFields(stream string, runs []streamRun) []field {
	var fields []field
	inRange := false
	for i, r := range runs {
		raw := stream[r.start:r.end]
		if inRange {
			if strings.Contains(raw, "{{end}}") {
				inRange = false
			}
			continue
		}
		if ri := strings.Index(raw, "{{range"); ri >= 0 {
			collectFields(raw[:ri], r, i, &fields)
			if ei := strings.Index(raw[ri:], "{{end}}"); ei >= 0 {
				collectFields(raw[ri+ei+len("{{end}}"):], r, i, &fields)
			} else {
				inRange = true
			}
			continue
		}
		collectFields(raw, r, i, &fields)
	}
	return fields
}

// collectFields достаёт поля из среза операнда и дописывает их в список.
func collectFields(slice string, r streamRun, runIdx int, fields *[]field) {
	for _, m := range placeholderRe.FindAllStringSubmatchIndex(slice, -1) {
		bodyStart, bodyEnd := m[2], m[3]
		*fields = append(*fields, field{
			key:       strings.TrimSpace(slice[bodyStart:bodyEnd]),
			begin:     r.start + m[0],
			end:       r.start + m[1],
			font:      r.font,
			size:      r.size,
			placeText: slice[bodyStart:bodyEnd],
			run:       runIdx,
		})
	}
}
