package pdfpl

import (
	"math"
	"strconv"
	"strings"
)

// findTmToken находит оператор позиционирования Tm/Td/TD, действующий на момент
// показа рана r (ближайший перед показом). Возвращает индекс токена операнда X,
// оба значения (X, Y) и ok — нашёлся ли оператор.
func findTmToken(toks []token, r streamRun) (xIdx int, x, y float64, ok bool) {
	for p := r.opTok - 1; p >= 0; p-- {
		if toks[p].kind != tokOp {
			continue
		}
		switch toks[p].text {
		case "Tm", "Td", "TD":
			if p < 2 || toks[p-1].kind != tokNum || toks[p-2].kind != tokNum {
				return 0, 0, 0, false
			}
			ex, errX := strconv.ParseFloat(toks[p-2].text, 64)
			ey, errY := strconv.ParseFloat(toks[p-1].text, 64)
			if errX != nil || errY != nil {
				return 0, 0, 0, false
			}
			return p - 2, ex, ey, true
		}
	}
	return 0, 0, 0, false
}

// penXY возвращает координаты начала рана r — операнды X, Y оператора Tm/Td/TD
// перед показом (MS Word позиционирует каждый фрагмент абсолютно).
func penXY(toks []token, r streamRun) (x, y float64, ok bool) {
	_, x, y, ok = findTmToken(toks, r)
	return x, y, ok
}

// penX возвращает горизонтальную координату начала рана r (операнд X оператора
// Tm/Td/TD перед показом).
func penX(toks []token, r streamRun) (float64, bool) {
	x, _, ok := penXY(toks, r)
	return x, ok
}

// tmXToken возвращает индекс токена горизонтального операнда X оператора
// Tm/Td/TD, действующего на момент показа рана (для переноса его позиции).
func tmXToken(toks []token, r streamRun) (int, bool) {
	xIdx, _, _, ok := findTmToken(toks, r)
	return xIdx, ok
}

// fmtNum форматирует число для вставки в поток без хвостовых нулей.
func fmtNum(v float64) string {
	s := strconv.FormatFloat(v, 'f', 3, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" || s == "-" {
		s = "0"
	}
	return s
}

// voidOverlappedSpaces обнуляет whitespace-only раны сразу за полем (тот же
// базлайн), перекрывающие горизонтальный интервал [leftX, rightX]. Такие раны —
// хвостовые пробелы Word-клетки: после внедрения более широкого значения они
// рисуются поверх текста и выглядят «разъехавшимся» кернингом.
func voidOverlappedSpaces(runs []streamRun, toks []token, lastRun int, y, leftX, rightX float64) []editRange {
	var edits []editRange
	for i := lastRun + 1; i < len(runs); i++ {
		r := runs[i]
		if strings.TrimSpace(r.text) != "" {
			break // следующий значимый текст — всё, что было, уже пробелы
		}
		x, yy, ok := penXY(toks, r)
		if !ok {
			break
		}
		if math.Abs(yy-y) > 0.01 {
			break // другой базлайн
		}
		if x >= rightX-0.5 {
			break // раны уже вне значения
		}
		edits = append(edits, editRange{start: r.start, end: r.end, to: "[() 0] TJ"})
	}
	return edits
}

// lineShiftEdits смещает Tm-x всех ранов строки (общий базлайн) на половину
// разницы ширин плейсхолдера и внедрённого значения, если значение стало шире
// и строка горизонтально отцентрирована вокруг области текста страницы — так
// Word рефлоуит центрированные абзацы. Для лево-выровненных строк возвращает
// nil и нулевой сдвиг, чтобы якорение осталось левым.
func lineShiftEdits(runs []streamRun, toks []token, baselineY, origPts, newPts float64, codec RunCodec) ([]editRange, float64) {
	dx := (origPts - newPts) / 2
	if dx >= 0 {
		// Значение не шире исходника — не трогаем.
		return nil, 0
	}

	lineIdx, minX, maxEnd, ok := lineBounds(runs, toks, baselineY, codec)
	if !ok {
		return nil, 0
	}
	lineCX := minX + (maxEnd-minX)/2

	pmn, pmx, ok := pageBounds(runs, toks, codec)
	if !ok {
		return nil, 0
	}
	pageCX := (pmn + pmx) / 2
	if math.Abs(lineCX-pageCX) > 30 {
		// Строка в колонке или выровнена влево — сдвигать нельзя.
		return nil, 0
	}

	var edits []editRange
	for _, i := range lineIdx {
		x0, _, ok := penXY(toks, runs[i])
		if !ok {
			continue
		}
		xIdx, ok := tmXToken(toks, runs[i])
		if !ok {
			continue
		}
		edits = append(edits, editRange{start: toks[xIdx].start, end: toks[xIdx].end, to: fmtNum(x0 + dx)})
	}
	return edits, dx
}

// lineBounds находит раны строки (общий базлайн baselineY) и её горизонтальные
// границы: список индексов, minX начала и maxEnd конца (в пикселях).
func lineBounds(runs []streamRun, toks []token, baselineY float64, codec RunCodec) (lineIdx []int, minX, maxEnd float64, ok bool) {
	minX, maxEnd = math.Inf(1), math.Inf(-1)
	for i, r := range runs {
		x0, y, ok := penXY(toks, r)
		// 0.01 — допуск совпадения базлайна в пунктах: исходные координаты Tm
		// записаны с точностью до сотых, парсинг воспроизводит их один в один.
		if !ok || math.Abs(y-baselineY) > 0.01 {
			continue
		}
		lineIdx = append(lineIdx, i)
		if x0 < minX {
			minX = x0
		}
		if e := x0 + runTextWidth(codec, r.text)*r.size/1000; e > maxEnd {
			maxEnd = e
		}
	}
	return lineIdx, minX, maxEnd, len(lineIdx) > 0
}

// pageBounds — горизонтальные границы всех ранов страницы.
func pageBounds(runs []streamRun, toks []token, codec RunCodec) (pmn, pmx float64, ok bool) {
	pmn, pmx = math.Inf(1), math.Inf(-1)
	for _, r := range runs {
		x0, _, ok := penXY(toks, r)
		if !ok {
			continue
		}
		if x0 < pmn {
			pmn = x0
		}
		if e := x0 + runTextWidth(codec, r.text)*r.size/1000; e > pmx {
			pmx = e
		}
	}
	return pmn, pmx, !math.IsInf(pmn, 1) && !math.IsInf(pmx, -1)
}

// runTextWidth — ширина текста рана: точная по кодекам (исходный шрифт),
// фолбэк textWidth для кодека с нулевыми ширинами (originalCodec).
func runTextWidth(codec RunCodec, s string) float64 {
	if w := srcWidth(codec, s); w > 0 {
		return w
	}
	return textWidth(s)
}

func fitLine(runs []streamRun, toks []token, lastRun int, x0, y0, origPts, advPt float64, alone bool, codec RunCodec) []editRange {
	var edits []editRange
	dx := 0.0
	if alone {
		shifts, d := lineShiftEdits(runs, toks, y0, origPts, advPt, codec)
		if len(shifts) > 0 {
			edits = append(edits, shifts...)
			dx = d
		}
	}
	fieldLeft := x0 + dx
	edits = append(edits, voidOverlappedSpaces(runs, toks, lastRun, y0, fieldLeft, fieldLeft+advPt)...)
	return edits
}

// lineAlone — единственное ли поле cf непустое содержимое строки базлайна y0:
// прочие раны того же базлайна допустимы только whitespace-пустыми.
func lineAlone(runs []streamRun, toks []token, y0 float64, cf *crossRunField) bool {
	partRuns := make(map[int]bool, len(cf.parts))
	for _, p := range cf.parts {
		partRuns[p.run] = true
	}
	for i, r := range runs {
		if partRuns[i] {
			continue
		}
		_, y, ok := penXY(toks, r)
		if !ok || math.Abs(y-y0) > 0.01 {
			continue
		}
		if strings.TrimSpace(r.text) != "" {
			return false
		}
	}
	return true
}

// crossRunLineFit доводит линию межранового поля под внедряемое значение.
// Центрирует строку (рефлоу Word) только когда поле — единственное непустое
// содержимое строки; иначе строка остаётся лево-якорённой, как в шаблоне.
func crossRunLineFit(runs []streamRun, toks []token, cf *crossRunField, codec RunCodec) []editRange {
	if codec == nil || len(cf.parts) == 0 {
		return nil
	}
	firstRun := runs[cf.parts[0].run]
	lastRunIdx := cf.parts[len(cf.parts)-1].run
	lastRun := runs[lastRunIdx]
	size := firstRun.size
	newPts := dstWidth(codec, cf.value) * size / 1000
	x0, y0, ok := penXY(toks, firstRun)
	if !ok {
		return nil
	}
	xL, _, ok := penXY(toks, lastRun)
	if !ok {
		return nil
	}
	// Геометрическая ширина плейсхолдера: от начала первого фрагмента до конца
	// последнего (Tm + ширина последнего в оригинальном шрифте).
	lastPts := srcWidth(codec, cf.parts[len(cf.parts)-1].text) * size / 1000
	origPts := (xL + lastPts) - x0
	return fitLine(runs, toks, lastRunIdx, x0, y0, origPts, newPts, lineAlone(runs, toks, y0, cf), codec)
}

// fitSingleLine доводит линию однорановой замены: центрирует строку при рефлоу
// (ран — чистый плейсхолдер, занимающий строку) и обнуляет whitespace-хвосты,
// перекрывающие внедрённое значение.
func fitSingleLine(runs []streamRun, toks []token, i int, r streamRun, advPt float64, codec RunCodec) []editRange {
	if codec == nil {
		return nil
	}
	x0, y0, ok := penXY(toks, r)
	if !ok {
		return nil
	}
	origPts := srcWidth(codec, r.text) * r.size / 1000
	alone := false
	if m := placeholderRe.FindStringIndex(r.text); m != nil && m[0] == 0 && m[1] == len(r.text) {
		alone = true
	}
	return fitLine(runs, toks, i, x0, y0, origPts, advPt, alone, codec)
}

// cellRect находит клип-прямоугольник (поле Word-клетки), содержащий начало
// рана r: последний оператор re (x y w h re W* n) перед оператором показа.
// Возвращает координаты поля и индекс токена его ширины w (для расширения).
func cellRect(toks []token, r streamRun) (cx, cy, cw, ch float64, wTok int, ok bool) {
	for p := r.opTok - 1; p >= 0; p-- {
		if toks[p].kind != tokOp || toks[p].text != "re" {
			continue
		}
		if p < 4 {
			continue
		}
		nums := make([]float64, 4)
		good := true
		for k := 0; k < 4; k++ {
			if toks[p-4+k].kind != tokNum {
				good = false
				break
			}
			n, err := strconv.ParseFloat(toks[p-4+k].text, 64)
			if err != nil {
				good = false
				break
			}
			nums[k] = n
		}
		if !good {
			continue
		}
		// Поле — тот прямоугольник, в который попадает начало рана.
		x0, xok := penX(toks, r)
		if !xok {
			continue
		}
		cx, cy, cw, ch = nums[0], nums[1], nums[2], nums[3]
		if x0 >= cx-0.5 && x0 <= cx+cw+0.5 {
			return cx, cy, cw, ch, p - 2, true
		}
	}
	return 0, 0, 0, 0, 0, false
}

// planFieldWiden расширяет поле (клип-прямоугольник re), в котором рисуется
// заменяемый ран, до натуральной ширины текста advPt (в точках от начала рана).
// Нужно, чтобы длинное значение не обрезалось у правой границы поля и буквы
// не налезали друг на друга. Возвращает nil, если поле уже вмещает текст или
// клип-бокса для рана нет (например, полностраничная обрезка).
func planFieldWiden(toks []token, r streamRun, advPt float64) []editRange {
	if advPt <= 0 {
		return nil
	}
	cx, _, cw, _, wTok, ok := cellRect(toks, r)
	if !ok {
		return nil
	}
	x0, ok := penX(toks, r)
	if !ok {
		return nil
	}
	// Правая граница, до которой текст должен читаться целиком (+запас 1pt).
	need := x0 + advPt + 1
	if right := cx + cw; need <= right {
		return nil
	}
	return []editRange{{start: toks[wTok].start, end: toks[wTok].end, to: strconv.FormatFloat(need-cx, 'f', 2, 64)}}
}

// padReplace заменяет срез [lo, hi) строки s на replacement.
func padReplace(s string, lo, hi int, replacement string) string {
	return s[:lo] + replacement + s[hi:]
}
