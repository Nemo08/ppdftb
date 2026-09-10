package pdfpl

import (
	"math"
	"strconv"
	"strings"
)

// findTmToken ╨╜╨░╤Е╨╛╨┤╨╕╤В ╨╛╨┐╨╡╤А╨░╤В╨╛╤А ╨┐╨╛╨╖╨╕╤Ж╨╕╨╛╨╜╨╕╤А╨╛╨▓╨░╨╜╨╕╤П Tm/Td/TD, ╨┤╨╡╨╣╤Б╤В╨▓╤Г╤О╤Й╨╕╨╣ ╨╜╨░ ╨╝╨╛╨╝╨╡╨╜╤В
// ╨┐╨╛╨║╨░╨╖╨░ ╤А╨░╨╜╨░ r (╨▒╨╗╨╕╨╢╨░╨╣╤И╨╕╨╣ ╨┐╨╡╤А╨╡╨┤ ╨┐╨╛╨║╨░╨╖╨╛╨╝). ╨Т╨╛╨╖╨▓╤А╨░╤Й╨░╨╡╤В ╨╕╨╜╨┤╨╡╨║╤Б ╤В╨╛╨║╨╡╨╜╨░ ╨╛╨┐╨╡╤А╨░╨╜╨┤╨░ X,
// ╨╛╨▒╨░ ╨╖╨╜╨░╤З╨╡╨╜╨╕╤П (X, Y) ╨╕ ok тАФ ╨╜╨░╤И╤С╨╗╤Б╤П ╨╗╨╕ ╨╛╨┐╨╡╤А╨░╤В╨╛╤А.
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

// penXY ╨▓╨╛╨╖╨▓╤А╨░╤Й╨░╨╡╤В ╨║╨╛╨╛╤А╨┤╨╕╨╜╨░╤В╤Л ╨╜╨░╤З╨░╨╗╨░ ╤А╨░╨╜╨░ r тАФ ╨╛╨┐╨╡╤А╨░╨╜╨┤╤Л X, Y ╨╛╨┐╨╡╤А╨░╤В╨╛╤А╨░ Tm/Td/TD
// ╨┐╨╡╤А╨╡╨┤ ╨┐╨╛╨║╨░╨╖╨╛╨╝ (MS Word ╨┐╨╛╨╖╨╕╤Ж╨╕╨╛╨╜╨╕╤А╤Г╨╡╤В ╨║╨░╨╢╨┤╤Л╨╣ ╤Д╤А╨░╨│╨╝╨╡╨╜╤В ╨░╨▒╤Б╨╛╨╗╤О╤В╨╜╨╛).
func penXY(toks []token, r streamRun) (x, y float64, ok bool) {
	_, x, y, ok = findTmToken(toks, r)
	return x, y, ok
}

// penX ╨▓╨╛╨╖╨▓╤А╨░╤Й╨░╨╡╤В ╨│╨╛╤А╨╕╨╖╨╛╨╜╤В╨░╨╗╤М╨╜╤Г╤О ╨║╨╛╨╛╤А╨┤╨╕╨╜╨░╤В╤Г ╨╜╨░╤З╨░╨╗╨░ ╤А╨░╨╜╨░ r (╨╛╨┐╨╡╤А╨░╨╜╨┤ X ╨╛╨┐╨╡╤А╨░╤В╨╛╤А╨░
// Tm/Td/TD ╨┐╨╡╤А╨╡╨┤ ╨┐╨╛╨║╨░╨╖╨╛╨╝).
func penX(toks []token, r streamRun) (float64, bool) {
	x, _, ok := penXY(toks, r)
	return x, ok
}

// tmXToken ╨▓╨╛╨╖╨▓╤А╨░╤Й╨░╨╡╤В ╨╕╨╜╨┤╨╡╨║╤Б ╤В╨╛╨║╨╡╨╜╨░ ╨│╨╛╤А╨╕╨╖╨╛╨╜╤В╨░╨╗╤М╨╜╨╛╨│╨╛ ╨╛╨┐╨╡╤А╨░╨╜╨┤╨░ X ╨╛╨┐╨╡╤А╨░╤В╨╛╤А╨░
// Tm/Td/TD, ╨┤╨╡╨╣╤Б╤В╨▓╤Г╤О╤Й╨╡╨│╨╛ ╨╜╨░ ╨╝╨╛╨╝╨╡╨╜╤В ╨┐╨╛╨║╨░╨╖╨░ ╤А╨░╨╜╨░ (╨┤╨╗╤П ╨┐╨╡╤А╨╡╨╜╨╛╤Б╨░ ╨╡╨│╨╛ ╨┐╨╛╨╖╨╕╤Ж╨╕╨╕).
func tmXToken(toks []token, r streamRun) (int, bool) {
	xIdx, _, _, ok := findTmToken(toks, r)
	return xIdx, ok
}

// fmtNum ╤Д╨╛╤А╨╝╨░╤В╨╕╤А╤Г╨╡╤В ╤З╨╕╤Б╨╗╨╛ ╨┤╨╗╤П ╨▓╤Б╤В╨░╨▓╨║╨╕ ╨▓ ╨┐╨╛╤В╨╛╨║ ╨▒╨╡╨╖ ╤Е╨▓╨╛╤Б╤В╨╛╨▓╤Л╤Е ╨╜╤Г╨╗╨╡╨╣.
func fmtNum(v float64) string {
	s := strconv.FormatFloat(v, 'f', 3, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" || s == "-" {
		s = "0"
	}
	return s
}

// voidOverlappedSpaces ╨╛╨▒╨╜╤Г╨╗╤П╨╡╤В whitespace-only ╤А╨░╨╜╤Л ╤Б╤А╨░╨╖╤Г ╨╖╨░ ╨┐╨╛╨╗╨╡╨╝ (╤В╨╛╤В ╨╢╨╡
// ╨▒╨░╨╖╨╗╨░╨╣╨╜), ╨┐╨╡╤А╨╡╨║╤А╤Л╨▓╨░╤О╤Й╨╕╨╡ ╨│╨╛╤А╨╕╨╖╨╛╨╜╤В╨░╨╗╤М╨╜╤Л╨╣ ╨╕╨╜╤В╨╡╤А╨▓╨░╨╗ [leftX, rightX]. ╨в╨░╨║╨╕╨╡ ╤А╨░╨╜╤Л тАФ
// ╤Е╨▓╨╛╤Б╤В╨╛╨▓╤Л╨╡ ╨┐╤А╨╛╨▒╨╡╨╗╤Л Word-╨║╨╗╨╡╤В╨║╨╕: ╨┐╨╛╤Б╨╗╨╡ ╨▓╨╜╨╡╨┤╤А╨╡╨╜╨╕╤П ╨▒╨╛╨╗╨╡╨╡ ╤И╨╕╤А╨╛╨║╨╛╨│╨╛ ╨╖╨╜╨░╤З╨╡╨╜╨╕╤П ╨╛╨╜╨╕
// ╤А╨╕╤Б╤Г╤О╤В╤Б╤П ╨┐╨╛╨▓╨╡╤А╤Е ╤В╨╡╨║╤Б╤В╨░ ╨╕ ╨▓╤Л╨│╨╗╤П╨┤╤П╤В ┬л╤А╨░╨╖╤К╨╡╤Е╨░╨▓╤И╨╕╨╝╤Б╤П┬╗ ╨║╨╡╤А╨╜╨╕╨╜╨│╨╛╨╝.
func voidOverlappedSpaces(runs []streamRun, toks []token, lastRun int, y, leftX, rightX float64) []editRange {
	var edits []editRange
	for i := lastRun + 1; i < len(runs); i++ {
		r := runs[i]
		if strings.TrimSpace(r.text) != "" {
			break // ╤Б╨╗╨╡╨┤╤Г╤О╤Й╨╕╨╣ ╨╖╨╜╨░╤З╨╕╨╝╤Л╨╣ ╤В╨╡╨║╤Б╤В тАФ ╨▓╤Б╤С, ╤З╤В╨╛ ╨▒╤Л╨╗╨╛, ╤Г╨╢╨╡ ╨┐╤А╨╛╨▒╨╡╨╗╤Л
		}
		x, yy, ok := penXY(toks, r)
		if !ok {
			break
		}
		if math.Abs(yy-y) > 0.01 {
			break // ╨┤╤А╤Г╨│╨╛╨╣ ╨▒╨░╨╖╨╗╨░╨╣╨╜
		}
		if x >= rightX-0.5 {
			break // ╤А╨░╨╜╤Л ╤Г╨╢╨╡ ╨▓╨╜╨╡ ╨╖╨╜╨░╤З╨╡╨╜╨╕╤П
		}
		edits = append(edits, editRange{start: r.start, end: r.end, to: "[() 0] TJ"})
	}
	return edits
}

// lineShiftEdits ╤Б╨╝╨╡╤Й╨░╨╡╤В Tm-x ╨▓╤Б╨╡╤Е ╤А╨░╨╜╨╛╨▓ ╤Б╤В╤А╨╛╨║╨╕ (╨╛╨▒╤Й╨╕╨╣ ╨▒╨░╨╖╨╗╨░╨╣╨╜) ╨╜╨░ ╨┐╨╛╨╗╨╛╨▓╨╕╨╜╤Г
// ╤А╨░╨╖╨╜╨╕╤Ж╤Л ╤И╨╕╤А╨╕╨╜ ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╨░ ╨╕ ╨▓╨╜╨╡╨┤╤А╤С╨╜╨╜╨╛╨│╨╛ ╨╖╨╜╨░╤З╨╡╨╜╨╕╤П, ╨╡╤Б╨╗╨╕ ╨╖╨╜╨░╤З╨╡╨╜╨╕╨╡ ╤Б╤В╨░╨╗╨╛ ╤И╨╕╤А╨╡
// ╨╕ ╤Б╤В╤А╨╛╨║╨░ ╨│╨╛╤А╨╕╨╖╨╛╨╜╤В╨░╨╗╤М╨╜╨╛ ╨╛╤В╤Ж╨╡╨╜╤В╤А╨╕╤А╨╛╨▓╨░╨╜╨░ ╨▓╨╛╨║╤А╤Г╨│ ╨╛╨▒╨╗╨░╤Б╤В╨╕ ╤В╨╡╨║╤Б╤В╨░ ╤Б╤В╤А╨░╨╜╨╕╤Ж╤Л тАФ ╤В╨░╨║
// Word ╤А╨╡╤Д╨╗╨╛╤Г╨╕╤В ╤Ж╨╡╨╜╤В╤А╨╕╤А╨╛╨▓╨░╨╜╨╜╤Л╨╡ ╨░╨▒╨╖╨░╤Ж╤Л. ╨Ф╨╗╤П ╨╗╨╡╨▓╨╛-╨▓╤Л╤А╨╛╨▓╨╜╨╡╨╜╨╜╤Л╤Е ╤Б╤В╤А╨╛╨║ ╨▓╨╛╨╖╨▓╤А╨░╤Й╨░╨╡╤В
// nil ╨╕ ╨╜╤Г╨╗╨╡╨▓╨╛╨╣ ╤Б╨┤╨▓╨╕╨│, ╤З╤В╨╛╨▒╤Л ╤П╨║╨╛╤А╨╡╨╜╨╕╨╡ ╨╛╤Б╤В╨░╨╗╨╛╤Б╤М ╨╗╨╡╨▓╤Л╨╝.
func lineShiftEdits(runs []streamRun, toks []token, baselineY, origPts, newPts float64) ([]editRange, float64) {
	dx := (origPts - newPts) / 2
	if dx >= 0 {
		// Значение не шире исходника — не трогаем.
		return nil, 0
	}

	lineIdx, minX, maxEnd, ok := lineBounds(runs, toks, baselineY)
	if !ok {
		return nil, 0
	}
	lineCX := minX + (maxEnd-minX)/2

	pmn, pmx, ok := pageBounds(runs, toks)
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
func lineBounds(runs []streamRun, toks []token, baselineY float64) (lineIdx []int, minX, maxEnd float64, ok bool) {
	minX, maxEnd = math.Inf(1), math.Inf(-1)
	for i, r := range runs {
		x0, y, ok := penXY(toks, r)
		if !ok || math.Abs(y-baselineY) > 0.01 {
			continue
		}
		lineIdx = append(lineIdx, i)
		if x0 < minX {
			minX = x0
		}
		if e := x0 + textWidth(r.text)*r.size/1000; e > maxEnd {
			maxEnd = e
		}
	}
	return lineIdx, minX, maxEnd, len(lineIdx) > 0
}

// pageBounds — горизонтальные границы всех ранов страницы.
func pageBounds(runs []streamRun, toks []token) (pmn, pmx float64, ok bool) {
	pmn, pmx = math.Inf(1), math.Inf(-1)
	for _, r := range runs {
		x0, _, ok := penXY(toks, r)
		if !ok {
			continue
		}
		if x0 < pmn {
			pmn = x0
		}
		if e := x0 + textWidth(r.text)*r.size/1000; e > pmx {
			pmx = e
		}
	}
	return pmn, pmx, !math.IsInf(pmn, 1) && !math.IsInf(pmx, -1)
}

func fitLine(runs []streamRun, toks []token, lastRun int, x0, y0, origPts, advPt float64, alone bool) []editRange {
	var edits []editRange
	dx := 0.0
	if alone {
		shifts, d := lineShiftEdits(runs, toks, y0, origPts, advPt)
		if len(shifts) > 0 {
			edits = append(edits, shifts...)
			dx = d
		}
	}
	fieldLeft := x0 + dx
	edits = append(edits, voidOverlappedSpaces(runs, toks, lastRun, y0, fieldLeft, fieldLeft+advPt)...)
	return edits
}

// lineAlone тАФ ╨╡╨┤╨╕╨╜╤Б╤В╨▓╨╡╨╜╨╜╨╛╨╡ ╨╗╨╕ ╨┐╨╛╨╗╨╡ cf ╨╜╨╡╨┐╤Г╤Б╤В╨╛╨╡ ╤Б╨╛╨┤╨╡╤А╨╢╨╕╨╝╨╛╨╡ ╤Б╤В╤А╨╛╨║╨╕ ╨▒╨░╨╖╨╗╨░╨╣╨╜╨░ y0:
// ╨┐╤А╨╛╤З╨╕╨╡ ╤А╨░╨╜╤Л ╤В╨╛╨│╨╛ ╨╢╨╡ ╨▒╨░╨╖╨╗╨░╨╣╨╜╨░ ╨┤╨╛╨┐╤Г╤Б╤В╨╕╨╝╤Л ╤В╨╛╨╗╤М╨║╨╛ whitespace-╨┐╤Г╤Б╤В╤Л╨╝╨╕.
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

// crossRunLineFit ╨┤╨╛╨▓╨╛╨┤╨╕╤В ╨╗╨╕╨╜╨╕╤О ╨╝╨╡╨╢╤А╨░╨╜╨╛╨▓╨╛╨│╨╛ ╨┐╨╛╨╗╤П ╨┐╨╛╨┤ ╨▓╨╜╨╡╨┤╤А╤П╨╡╨╝╨╛╨╡ ╨╖╨╜╨░╤З╨╡╨╜╨╕╨╡.
// ╨ж╨╡╨╜╤В╤А╨╕╤А╤Г╨╡╤В ╤Б╤В╤А╨╛╨║╤Г (╤А╨╡╤Д╨╗╨╛╤Г Word) ╤В╨╛╨╗╤М╨║╨╛ ╨║╨╛╨│╨┤╨░ ╨┐╨╛╨╗╨╡ тАФ ╨╡╨┤╨╕╨╜╤Б╤В╨▓╨╡╨╜╨╜╨╛╨╡ ╨╜╨╡╨┐╤Г╤Б╤В╨╛╨╡
// ╤Б╨╛╨┤╨╡╤А╨╢╨╕╨╝╨╛╨╡ ╤Б╤В╤А╨╛╨║╨╕; ╨╕╨╜╨░╤З╨╡ ╤Б╤В╤А╨╛╨║╨░ ╨╛╤Б╤В╨░╤С╤В╤Б╤П ╨╗╨╡╨▓╨╛-╤П╨║╨╛╤А╤С╨╜╨╜╨╛╨╣, ╨║╨░╨║ ╨▓ ╤И╨░╨▒╨╗╨╛╨╜╨╡.
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
	// ╨У╨╡╨╛╨╝╨╡╤В╤А╨╕╤З╨╡╤Б╨║╨░╤П ╤И╨╕╤А╨╕╨╜╨░ ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╨░: ╨╛╤В ╨╜╨░╤З╨░╨╗╨░ ╨┐╨╡╤А╨▓╨╛╨│╨╛ ╤Д╤А╨░╨│╨╝╨╡╨╜╤В╨░ ╨┤╨╛ ╨║╨╛╨╜╤Ж╨░
	// ╨┐╨╛╤Б╨╗╨╡╨┤╨╜╨╡╨│╨╛ (Tm + ╤И╨╕╤А╨╕╨╜╨░ ╨┐╨╛╤Б╨╗╨╡╨┤╨╜╨╡╨│╨╛ ╨▓ ╨╛╤А╨╕╨│╨╕╨╜╨░╨╗╤М╨╜╨╛╨╝ ╤И╤А╨╕╤Д╤В╨╡).
	lastPts := srcWidth(codec, cf.parts[len(cf.parts)-1].text) * size / 1000
	origPts := (xL + lastPts) - x0
	return fitLine(runs, toks, lastRunIdx, x0, y0, origPts, newPts, lineAlone(runs, toks, y0, cf))
}

// fitSingleLine ╨┤╨╛╨▓╨╛╨┤╨╕╤В ╨╗╨╕╨╜╨╕╤О ╨╛╨┤╨╜╨╛╤А╨░╨╜╨╛╨▓╨╛╨╣ ╨╖╨░╨╝╨╡╨╜╤Л: ╤Ж╨╡╨╜╤В╤А╨╕╤А╤Г╨╡╤В ╤Б╤В╤А╨╛╨║╤Г ╨┐╤А╨╕ ╤А╨╡╤Д╨╗╨╛╤Г
// (╤А╨░╨╜ тАФ ╤З╨╕╤Б╤В╤Л╨╣ ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А, ╨╖╨░╨╜╨╕╨╝╨░╤О╤Й╨╕╨╣ ╤Б╤В╤А╨╛╨║╤Г) ╨╕ ╨╛╨▒╨╜╤Г╨╗╤П╨╡╤В whitespace-╤Е╨▓╨╛╤Б╤В╤Л,
// ╨┐╨╡╤А╨╡╨║╤А╤Л╨▓╨░╤О╤Й╨╕╨╡ ╨▓╨╜╨╡╨┤╤А╤С╨╜╨╜╨╛╨╡ ╨╖╨╜╨░╤З╨╡╨╜╨╕╨╡.
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
	return fitLine(runs, toks, i, x0, y0, origPts, advPt, alone)
}

// cellRect ╨╜╨░╤Е╨╛╨┤╨╕╤В ╨║╨╗╨╕╨┐-╨┐╤А╤П╨╝╨╛╤Г╨│╨╛╨╗╤М╨╜╨╕╨║ (╨┐╨╛╨╗╨╡ Word-╨║╨╗╨╡╤В╨║╨╕), ╤Б╨╛╨┤╨╡╤А╨╢╨░╤Й╨╕╨╣ ╨╜╨░╤З╨░╨╗╨╛
// ╤А╨░╨╜╨░ r: ╨┐╨╛╤Б╨╗╨╡╨┤╨╜╨╕╨╣ ╨╛╨┐╨╡╤А╨░╤В╨╛╤А re (x y w h re W* n) ╨┐╨╡╤А╨╡╨┤ ╨╛╨┐╨╡╤А╨░╤В╨╛╤А╨╛╨╝ ╨┐╨╛╨║╨░╨╖╨░.
// ╨Т╨╛╨╖╨▓╤А╨░╤Й╨░╨╡╤В ╨║╨╛╨╛╤А╨┤╨╕╨╜╨░╤В╤Л ╨┐╨╛╨╗╤П ╨╕ ╨╕╨╜╨┤╨╡╨║╤Б ╤В╨╛╨║╨╡╨╜╨░ ╨╡╨│╨╛ ╤И╨╕╤А╨╕╨╜╤Л w (╨┤╨╗╤П ╤А╨░╤Б╤И╨╕╤А╨╡╨╜╨╕╤П).
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
		// ╨Я╨╛╨╗╨╡ тАФ ╤В╨╛╤В ╨┐╤А╤П╨╝╨╛╤Г╨│╨╛╨╗╤М╨╜╨╕╨║, ╨▓ ╨║╨╛╤В╨╛╤А╤Л╨╣ ╨┐╨╛╨┐╨░╨┤╨░╨╡╤В ╨╜╨░╤З╨░╨╗╨╛ ╤А╨░╨╜╨░.
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

// planFieldWiden ╤А╨░╤Б╤И╨╕╤А╤П╨╡╤В ╨┐╨╛╨╗╨╡ (╨║╨╗╨╕╨┐-╨┐╤А╤П╨╝╨╛╤Г╨│╨╛╨╗╤М╨╜╨╕╨║ re), ╨▓ ╨║╨╛╤В╨╛╤А╨╛╨╝ ╤А╨╕╤Б╤Г╨╡╤В╤Б╤П
// ╨╖╨░╨╝╨╡╨╜╤П╨╡╨╝╤Л╨╣ ╤А╨░╨╜, ╨┤╨╛ ╨╜╨░╤В╤Г╤А╨░╨╗╤М╨╜╨╛╨╣ ╤И╨╕╤А╨╕╨╜╤Л ╤В╨╡╨║╤Б╤В╨░ advPt (╨▓ ╤В╨╛╤З╨║╨░╤Е ╨╛╤В ╨╜╨░╤З╨░╨╗╨░ ╤А╨░╨╜╨░).
// ╨Э╤Г╨╢╨╜╨╛, ╤З╤В╨╛╨▒╤Л ╨┤╨╗╨╕╨╜╨╜╨╛╨╡ ╨╖╨╜╨░╤З╨╡╨╜╨╕╨╡ ╨╜╨╡ ╨╛╨▒╤А╨╡╨╖╨░╨╗╨╛╤Б╤М ╤Г ╨┐╤А╨░╨▓╨╛╨╣ ╨│╤А╨░╨╜╨╕╤Ж╤Л ╨┐╨╛╨╗╤П ╨╕ ╨▒╤Г╨║╨▓╤Л
// ╨╜╨╡ ╨╜╨░╨╗╨╡╨╖╨░╨╗╨╕ ╨┤╤А╤Г╨│ ╨╜╨░ ╨┤╤А╤Г╨│╨░. ╨Т╨╛╨╖╨▓╤А╨░╤Й╨░╨╡╤В nil, ╨╡╤Б╨╗╨╕ ╨┐╨╛╨╗╨╡ ╤Г╨╢╨╡ ╨▓╨╝╨╡╤Й╨░╨╡╤В ╤В╨╡╨║╤Б╤В ╨╕╨╗╨╕
// ╨║╨╗╨╕╨┐-╨▒╨╛╨║╤Б╨░ ╨┤╨╗╤П ╤А╨░╨╜╨░ ╨╜╨╡╤В (╨╜╨░╨┐╤А╨╕╨╝╨╡╤А, ╨┐╨╛╨╗╨╜╨╛╤Б╤В╤А╨░╨╜╨╕╤З╨╜╨░╤П ╨╛╨▒╤А╨╡╨╖╨║╨░).
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
	// ╨Я╤А╨░╨▓╨░╤П ╨│╤А╨░╨╜╨╕╤Ж╨░, ╨┤╨╛ ╨║╨╛╤В╨╛╤А╨╛╨╣ ╤В╨╡╨║╤Б╤В ╨┤╨╛╨╗╨╢╨╡╨╜ ╤З╨╕╤В╨░╤В╤М╤Б╤П ╤Ж╨╡╨╗╨╕╨║╨╛╨╝ (+╨╖╨░╨┐╨░╤Б 1pt).
	need := x0 + advPt + 1
	if right := cx + cw; need <= right {
		return nil
	}
	return []editRange{{start: toks[wTok].start, end: toks[wTok].end, to: strconv.FormatFloat(need-cx, 'f', 2, 64)}}
}

// padReplace ╨╖╨░╨╝╨╡╨╜╤П╨╡╤В ╤Б╤А╨╡╨╖ [lo, hi) ╤Б╤В╤А╨╛╨║╨╕ s ╨╜╨░ replacement.
func padReplace(s string, lo, hi int, replacement string) string {
	return s[:lo] + replacement + s[hi:]
}
