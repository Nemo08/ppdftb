package pdfpl

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// buildStream заменяет плейсхолдеры в контентном потоке значениями из values.
// Плейсхолдер, значение которого шире исходного текста, остаётся не тронутым.
// Замена сохраняет ширину текста: разница в ширине компенсируется числом
// кернинга в TJ-массиве, чтобы следующий текст не поехал.
//
// Обрабатывает и однорановые, и межрановые (cross-run) плейсхолдеры.
func buildStream(stream string, runs []streamRun, fields []field, values map[string]string) (string, error) {
	_, _, _, edits, err := buildStreamCounted(stream, runs, fields, values, false, nil)
	if err != nil {
		return "", err
	}
	return applyEdits(stream, edits), nil
}

// buildStreamCounted — то же, что buildStream, но дополнительно возвращает
// количество заменённых и пропущенных плейсхолдеров.
//
// allowWider=true разрешает замену значениями, шире исходного текста
// плейсхолдера (текст «выедет» вправо, за пределы исходной линии). При
// allowWider=false такие значения пропускаются (безопасное поведение по умолчанию).
//
// provider — необязательный провайдер кодеков для переключения ранов на
// внедрённые шрифты (файловый слой). Редакции возвращаются в координатах
// исходного потока и применяются вызывающей стороной справа налево.
func buildStreamCounted(stream string, runs []streamRun, fields []field, values map[string]string, allowWider bool, provider CodecProvider) (string, int, int, []editRange, error) {
	// Планируем все правки (операнды + переименования шрифтов) в координатах
	// исходного потока. Сам поток модифицируется только в Replace.
	toks, err := tokenize(stream)
	if err != nil {
		return "", 0, 0, nil, fmt.Errorf("разбор контентного потока: %w", err)
	}
	var edits []editRange

	// Межрановые плейсхолдеры заменяются на уровне ран — раны, участвующие
	// в них, исключаем из однорановой обработки. Межрановые плейсхолдеры
	// обрабатываем ВСЕГДА (даже когда fields пуст, т.к. findFields не видит
	// фрагменты {{. разнесённые по ранам).
	crossFields := buildCrossRunFields(runs, values)
	crossEdits, crossReplaced, crossSkipped := replaceCrossRunFields(runs, crossFields, allowWider, provider, toks)
	edits = append(edits, crossEdits...)

	if len(fields) == 0 {
		return stream, crossReplaced, crossSkipped, edits, nil
	}

	// Множество ранов, уже обработанных межрановой заменой.
	consumed := make(map[int]bool)
	for _, cf := range crossFields {
		if cf.value == "" || (!allowWider && textWidth(cf.value) > cf.origWidth) {
			continue
		}
		for _, part := range cf.parts {
			consumed[part.run] = true
		}
	}

	// Группируем однорановые поля по ранам — каждый ран пересобирается целиком.
	byRun := map[int][]field{}
	for _, f := range fields {
		if consumed[f.run] {
			continue
		}
		byRun[f.run] = append(byRun[f.run], f)
	}

	withinReplaced := 0
	withinSkipped := 0
	for i, r := range runs {
		fl := byRun[i]
		if len(fl) == 0 {
			continue
		}
		newOperand, codec, advPt, err := rebuildOperand(r, fl, values, allowWider, provider)
		if err != nil {
			return "", 0, 0, nil, err
		}
		if newOperand == "" {
			withinSkipped += len(fl)
			continue
		}
		withinReplaced += len(fl)
		edits = append(edits, editRange{start: r.start, end: r.end, to: newOperand})
		if codec != nil && codec.FontName() != "" {
			if st, en, ok := fontNameRange(toks, r); ok {
				edits = append(edits, editRange{start: st, end: en, to: codec.FontName()})
			}
			// Значение рисуется внедрённым шрифтом — расширяем поле (клип-клетку
			// Word), чтобы длинный текст не обрезался и буквы не налезали.
			edits = append(edits, planFieldWiden(toks, r, advPt)...)
			// Доводка линии: центрирование и обнуление перекрывающих пробелов.
			edits = append(edits, fitSingleLine(runs, toks, i, r, advPt, codec)...)
		}
	}
	return stream, crossReplaced + withinReplaced, crossSkipped + withinSkipped, edits, nil
}

// fontNameRange возвращает байтовый диапазон токена имени шрифта (/F1) в
// операторе Tf, действующем на момент показа рана.
func fontNameRange(toks []token, r streamRun) (int, int, bool) {
	n, _, ok := tfTokens(toks, r)
	if !ok {
		return 0, 0, false
	}
	return toks[n].start, toks[n].end, true
}

// tfTokens находит индексы токенов имени и размера в операторе Tf,
// действующем на момент показа рана (последний /F1 <size> Tf перед
// оператором показа).
func tfTokens(toks []token, r streamRun) (name, size int, ok bool) {
	for p := r.opTok - 1; p >= 0; p-- {
		if toks[p].kind != tokOp || toks[p].text != "Tf" {
			continue
		}
		if p < 2 || toks[p-1].kind != tokNum || toks[p-2].kind != tokName {
			continue
		}
		return p - 2, p - 1, true
	}
	return 0, 0, false
}

// penX возвращает горизонтальную координату начала рана r — предпоследний
// числовой операнд оператора Tm/Td/TD перед показом (MS Word позиционирует
// каждый фрагмент абсолютно: ... E F Tm, где E — горизонталь).
func penX(toks []token, r streamRun) (float64, bool) {
	x, _, ok := penXY(toks, r)
	return x, ok
}

// penXY возвращает координаты начала рана r — последний и предпоследний
// числовые операнды оператора Tm/Td/TD перед показом (X, Y).
func penXY(toks []token, r streamRun) (x, y float64, ok bool) {
	for p := r.opTok - 1; p >= 0; p-- {
		if toks[p].kind != tokOp {
			continue
		}
		switch toks[p].text {
		case "Tm", "Td", "TD":
			if p < 2 || toks[p-1].kind != tokNum || toks[p-2].kind != tokNum {
				return 0, 0, false
			}
			ex, errX := strconv.ParseFloat(toks[p-2].text, 64)
			ey, errY := strconv.ParseFloat(toks[p-1].text, 64)
			if errX != nil || errY != nil {
				return 0, 0, false
			}
			return ex, ey, true
		}
	}
	return 0, 0, false
}

// tmXToken возвращает индекс токена горизонтального операнда X оператора
// Tm/Td/TD, действующего на момент показа рана (для переноса его позиции).
func tmXToken(toks []token, r streamRun) (int, bool) {
	for p := r.opTok - 1; p >= 0; p-- {
		if toks[p].kind != tokOp {
			continue
		}
		switch toks[p].text {
		case "Tm", "Td", "TD":
			if p < 2 || toks[p-1].kind != tokNum || toks[p-2].kind != tokNum {
				return 0, false
			}
			return p - 2, true
		}
	}
	return 0, false
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
func lineShiftEdits(runs []streamRun, toks []token, baselineY, origPts, newPts float64) ([]editRange, float64) {
	dx := (origPts - newPts) / 2
	if dx >= 0 {
		return nil, 0 // значение не шире исходника — не трогаем
	}
	var lineIdx []int
	minX, maxEnd := math.Inf(1), math.Inf(-1)
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
	if len(lineIdx) == 0 || math.IsInf(minX, 1) || math.IsInf(maxEnd, -1) {
		return nil, 0
	}
	lineCX := minX + (maxEnd-minX)/2
	pmn, pmx := math.Inf(1), math.Inf(-1)
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
	if math.IsInf(pmn, 1) || math.IsInf(pmx, -1) {
		return nil, 0
	}
	pageCX := (pmn + pmx) / 2
	if math.Abs(lineCX-pageCX) > 30 {
		return nil, 0 // строка в колонке или выровнена влево — сдвигать нельзя
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

// crossRunLineFit доводит линию межранового поля под внедряемое значение.
// Центрирует строку (рефлоу Word) только когда поле — единственное непустое
// содержимое строки; иначе строка остаётся лево-якорённой, как в шаблоне.
// В обоих случаях обнуляются whitespace-only раны, перекрывающие новое значение
// (иначе выглядит, как сбитый кернинг).
func crossRunLineFit(runs []streamRun, toks []token, cf *crossRunField, codec RunCodec) []editRange {
	var edits []editRange
	if codec == nil || len(cf.parts) == 0 {
		return edits
	}
	firstRun := runs[cf.parts[0].run]
	lastRun := runs[cf.parts[len(cf.parts)-1].run]
	size := firstRun.size
	newPts := dstWidth(codec, cf.value) * size / 1000
	x0, y0, ok := penXY(toks, firstRun)
	if !ok {
		return edits
	}
	xL, _, ok := penXY(toks, lastRun)
	if !ok {
		return edits
	}
	// Геометрическая ширина плейсхолдера: от начала первого фрагмента до конца
	// последнего (Tm + ширина последнего в оригинальном шрифте).
	lastPts := srcWidth(codec, cf.parts[len(cf.parts)-1].text) * size / 1000
	origPts := (xL + lastPts) - x0

	// Поле одно на строке? Непустые раны того же базлайна вне частей поля — нет.
	partRuns := make(map[int]bool, len(cf.parts))
	for _, p := range cf.parts {
		partRuns[p.run] = true
	}
	alone := true
	for i, r := range runs {
		if partRuns[i] {
			continue
		}
		_, y, ok := penXY(toks, r)
		if !ok || math.Abs(y-y0) > 0.01 {
			continue
		}
		if strings.TrimSpace(r.text) != "" {
			alone = false
			break
		}
	}

	dx := 0.0
	if alone {
		shifts, d := lineShiftEdits(runs, toks, y0, origPts, newPts)
		if len(shifts) > 0 {
			edits = append(edits, shifts...)
			dx = d
		}
	}
	fieldLeft := x0 + dx
	edits = append(edits, voidOverlappedSpaces(runs, toks, cf.parts[len(cf.parts)-1].run, y0, fieldLeft, fieldLeft+newPts)...)
	return edits
}

// fitSingleLine доводит линию однорановой замены: центрирует строку при рефлоу
// (ран — чистый плейсхолдер, занимающий строку) и обнуляет whitespace-хвосты,
// перекрывающие внедрённое значение.
func fitSingleLine(runs []streamRun, toks []token, i int, r streamRun, advPt float64, codec RunCodec) []editRange {
	var edits []editRange
	if codec == nil {
		return edits
	}
	x0, y0, ok := penXY(toks, r)
	if !ok {
		return edits
	}
	origPts := srcWidth(codec, r.text) * r.size / 1000
	dx := 0.0
	if m := placeholderRe.FindStringIndex(r.text); m != nil && m[0] == 0 && m[1] == len(r.text) {
		shifts, d := lineShiftEdits(runs, toks, y0, origPts, advPt)
		if len(shifts) > 0 {
			edits = append(edits, shifts...)
			dx = d
		}
	}
	fieldLeft := x0 + dx
	edits = append(edits, voidOverlappedSpaces(runs, toks, i, y0, fieldLeft, fieldLeft+advPt)...)
	return edits
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

// editRange — точечная правка байтового диапазона исходного потока.
// Все редакции планируются в координатах ИСХОДНОГО потока (до любых правок)
// и применяются в Replace справа налево по убыванию start, поэтому позиции
// не поехали даже при изменении длины ранних фрагментов.
type editRange struct {
	start, end int    // байтовый диапазон в исходном потоке
	to         string // текст замены
}

// RunCodec — пофайловое кодирование и измерение текста ран-блока при
// переключении рана на внедрённый шрифт (файловый слой ReplaceFile).
type RunCodec interface {
	// Encode возвращает содержимое строкового литерала (байты) для utf8-текста.
	Encode(utf8 string) string
	// Hex — true, если Encode возвращает байты двухбайтовой CID-строки
	// Identity-H шрифта; такой литерал печатается в форме <...>.
	Hex() bool
	// Width — ширина utf8 в тысячных долях em в ИСХОДНОМ шрифте рана
	// (метрики того шрифта, которым был нарисован плейсхолдер).
	Width(utf8 string) float64
	// NewWidth — ширина utf8 в тысячных долях em в новом (внедряемом) шрифте.
	NewWidth(utf8 string) float64
	// FontName — имя ресурса нового шрифта (/TF1) для подстановки в Tf, либо "".
	FontName() string
}

// CodecProvider выбирает кодировщик для ран-блока. font — имя ресурса
// активного шрифта рана (/F1), text — итоговый текст, который будет
// отрисован этим раном. nil — ран не переключать на другой шрифт
// (текст остаётся в исходном шрифте, ширина — эвристическая).
type CodecProvider func(font string, text string) RunCodec

// srcWidth возвращает ширину текста в исходном шрифте рана (или эвристику).
func srcWidth(c RunCodec, s string) float64 {
	if c != nil {
		return c.Width(s)
	}
	return textWidth(s)
}

// dstWidth возвращает ширину текста в новом (внедряемом) шрифте (или эвристику).
func dstWidth(c RunCodec, s string) float64 {
	if c != nil {
		return c.NewWidth(s)
	}
	return textWidth(s)
}

// hexEncode кодирует байты в шестнадцатеричную строку PDF-литерала.
func hexEncode(b []byte) string {
	const hexd = "0123456789ABCDEF"
	var sb strings.Builder
	for _, c := range b {
		sb.WriteByte(hexd[c>>4])
		sb.WriteByte(hexd[c&0x0f])
	}
	return sb.String()
}

// renderOperand собирает операнд оператора показа для итогового текста:
// string-литерал (...) либо hex-литерал <...> (для Identity-H CID-шрифтов),
// при необходимости с хвостовым кернингом компенсации ширины.
func renderOperand(text string, delta float64, codec RunCodec) string {
	payload := text
	if codec != nil {
		payload = codec.Encode(text)
	}
	var literal string
	if codec != nil && codec.Hex() {
		literal = "<" + hexEncode([]byte(payload)) + ">"
	} else {
		literal = "(" + quoteString(payload) + ")"
	}
	if delta > 0 {
		return fmt.Sprintf("[%s %g] TJ", literal, -delta)
	}
	return literal + " Tj"
}

// rebuildOperand пересобирает операнд ран-блока, заменяя поля значениями.
// Возвращает пустую строку, если замена не произошла (ключ отсутствует в values,
// или — при allowWider=false — значение шире плейсхолдера).
// Операнд рендерится как TJ-массив, если потребовалась компенсация ширины;
// иначе сохраняется строковая форма (... ) Tj.
//
// Если провайдер кодеков решит переключить ран на внедрённый шрифт, возвращается
// непустой codec (нужен вызывающей стороне для переименования Tf). Размер шрифта
// и кернинг контекста (Tc/Tz/TJ) не меняются: значение рендерится размером
// плейсхолдера, автоусадка под ширину бокса не выполняется.
//
// advPt — полная ширина итогового текста (finalText) в точках при размере r.size;
// нужна вызывающей стороне для расширения поля (клип-бокса), чтобы длинное
// значение не обрезалось и не налезало на соседние символы.
func rebuildOperand(r streamRun, fields []field, values map[string]string, allowWider bool, provider CodecProvider) (operand string, codec RunCodec, advPt float64, err error) {
	segs := segOfRun(r, fields)
	if len(segs) == 0 {
		return "", nil, 0, nil
	}

	// Собираем итоговый текст и проверяем наличие всех значений.
	var text strings.Builder
	for _, s := range segs {
		if s.kind == segText {
			text.WriteString(s.text)
			continue
		}
		val, ok := values[s.value]
		if !ok {
			return "", nil, 0, nil
		}
		text.WriteString(val)
	}
	finalText := text.String()

	// Переключение рана на внедрённый шрифт, если исходный шрифт не покрывает
	// итоговый текст (например, кириллические значения в ASCII-шрифте F1).
	if provider != nil {
		codec = provider(r.font, finalText)
	}

	// Проверяем суммарную дельту ширины и допустимость значения.
	var delta float64
	for _, s := range segs {
		if s.kind == segText {
			continue
		}
		val := values[s.value]
		// Значение шире плейсхолдера — замену отменяем целиком,
		// если только явно не разрешено (allowWider).
		if !allowWider && dstWidth(codec, val) > srcWidth(codec, s.text) {
			return "", nil, 0, nil
		}
		delta += srcWidth(codec, s.text) - dstWidth(codec, val)
	}

	return renderOperand(finalText, delta, codec), codec, dstWidth(codec, finalText) * r.size / 1000, nil
}

// segOfRun разбирает текст ран-блока на сегменты между плейсхолдерами.
type segKind uint8

const (
	segText segKind = iota
	segValue
)

type segment struct {
	kind  segKind
	text  string // для segText — неизменный текст; для segValue — полный плейсхолдер {{...}}
	begin int    // байтовая граница в рамках операнда (относительно r.start)
	end   int
	font  string
	size  float64
	value string // для segValue — ключ для поиска в values
}

// segOfRun режет логический текст рана на сегменты по границам плейсхолдеров,
// учитывая только те, что реально входят в список полей рана.
func segOfRun(r streamRun, fields []field) []segment {
	// Множество ключей полей рана: заменяем только то, что нашёл findFields
	// (плейсхолдеры внутри range-блоков сюда не попадают).
	known := make(map[string]bool, len(fields))
	for _, f := range fields {
		known[f.key] = true
	}
	var segs []segment
	pos := 0 // текущая позиция в r.text
	for _, m := range placeholderRe.FindAllStringSubmatchIndex(r.text, -1) {
		// Пропускаем плейсхолдеры, не принадлежащие списку полей (например,
		// из range-блоков, которые игнорируются).
		if m[0] < pos {
			continue
		}
		key := strings.TrimSpace(r.text[m[2]:m[3]])
		if !known[key] {
			// Не в списке — включаем как обычный текст, не пересобирая.
			segs = append(segs, segment{
				kind: segText,
				text: r.text[pos:m[1]],
			})
			pos = m[1]
			continue
		}
		// Хвост текста перед плейсхолдером.
		if m[0] > pos {
			segs = append(segs, segment{
				kind:  segText,
				text:  r.text[pos:m[0]],
				begin: pos,
				end:   m[0],
				font:  r.font,
				size:  r.size,
			})
		}
		// Сам плейсхолдер: ширина учитывается целиком вместе со скобками,
		// ведь именно он был отрисован в PDF и занимал место.
		segs = append(segs, segment{
			kind:  segValue,
			text:  r.text[m[0]:m[1]],
			begin: m[0],
			end:   m[1],
			font:  r.font,
			size:  r.size,
			value: key,
		})
		pos = m[1]
	}
	// Хвост текста после последнего плейсхолдера.
	if pos < len(r.text) {
		segs = append(segs, segment{
			kind:  segText,
			text:  r.text[pos:],
			begin: pos,
			end:   len(r.text),
			font:  r.font,
			size:  r.size,
		})
	}
	return segs
}

// quoteString экранирует строку в PDF-литерал (скобки и обратная косая).
func quoteString(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '(', ')', '\\':
			sb.WriteByte('\\')
			sb.WriteByte(c)
		default:
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

// crossRunContrib — фрагмент плейсхолдера, вкладываемый одним ран-блоком.
type crossRunContrib struct {
	run     int     // индекс рана в общем списке
	text    string  // фрагмент текста плейсхолдера в этом ране
	start   int     // абсолютное смещение начала фрагмента в потоке
	end     int     // абсолютное смещение конца фрагмента в потоке
	partial bool    // true = фрагмент неполный (только {{ или только }})
	font    string  // активный шрифт (/F1)
	size    float64 // размер шрифта из Tf
}

// crossRunField — плейсхолдер {{.Key}}, разнесённый по нескольким ран-блокам.
// Встречается в PDF, сгенерированных MS Word: каждый символ {{}} potentially
// оказывается в отдельном BT/ET-блоке.
type crossRunField struct {
	key       string            // имя ключа плейсхолдера
	value     string            // значение из values ("" = ключ не найден)
	parts     []crossRunContrib // вклады по ранам (по порядку появления)
	origWidth float64           // суммарная ширина исходных фрагментов в glyph-units
}

// buildCrossRunFields определяет плейсхолдеры, разнесённые по нескольким ранам.
// Алгоритм: конкатенируем тексты всех ранов, ищем {{...}} в полной строке
// (пропуская range-блоки), затем размаппингуем найденные позиции обратно
// на отдельные раны. Плейсхолдеры, полностью попадающие в один ран, пропускаются —
// их обрабатывает существующая within-run логика (segOfRun).
// Значение берётся из values по голому ключу; если ключа нет — value пустая,
// и такой плейсхолдер пропускается при замене.
func buildCrossRunFields(runs []streamRun, values map[string]string) []*crossRunField {
	if len(runs) < 2 {
		return nil
	}

	// Склеиваем тексты ранов и строим мапу «символ склейки → ран».
	var full strings.Builder
	type runSpan struct {
		run      int
		localOff int // смещение начала рана в full
		localEnd int // смещение конца рана в full (исключительно)
		absStart int // абсолютное смещение в потоке
		font     string
		size     float64
	}
	var spans []runSpan
	for i, r := range runs {
		start := full.Len()
		full.WriteString(r.text)
		spans = append(spans, runSpan{
			run:      i,
			localOff: start,
			localEnd: full.Len(),
			absStart: r.start,
			font:     r.font,
			size:     r.size,
		})
	}

	combined := full.String()
	var result []*crossRunField

	// Проходим по плейсхолдерам, отслеживая вхождение в range-блоки:
	// {{range ...}}...{{end}} игнорируются целиком.
	inRange := false
	i := 0
	for i < len(combined) {
		// Ищем открывающий или закрывающий тег range.
		if rel := strings.Index(combined[i:], "{{range"); rel >= 0 && (!inRange) {
			idx := i + rel
			if ri := placeholderRe.FindStringIndex(combined[idx:]); ri != nil && strings.HasPrefix(strings.TrimSpace(combined[idx+ri[0]:idx+ri[1]]), "{{range") {
				inRange = true
				i = idx + ri[1]
				continue
			}
		}
		if inRange {
			if rel := strings.Index(combined[i:], "{{end}}"); rel >= 0 {
				i += rel + len("{{end}}")
				inRange = false
				continue
			}
			// Ищем следующий плейсхолдер, чтобы не выйти за range.
			nxt := placeholderRe.FindStringIndex(combined[i:])
			if nxt == nil {
				break
			}
			i += nxt[0]
			continue
		}

		m := placeholderRe.FindStringSubmatchIndex(combined[i:])
		if m == nil {
			break
		}
		// Сдвигаем позиции на базовый offset i.
		matchStart, matchEnd := i+m[0], i+m[1]
		bodyStart, bodyEnd := i+m[2], i+m[3]

		// Определяем, какие раны участвуют в этом плейсхолдере.
		var contributing []crossRunContrib
		for _, sp := range spans {
			fragStart := matchStart
			if fragStart < sp.localOff {
				fragStart = sp.localOff
			}
			fragEnd := matchEnd
			if fragEnd > sp.localEnd {
				fragEnd = sp.localEnd
			}
			if fragStart >= fragEnd {
				continue
			}
			contributing = append(contributing, crossRunContrib{
				run:     sp.run,
				text:    combined[fragStart:fragEnd],
				start:   sp.absStart + (fragStart - sp.localOff),
				end:     sp.absStart + (fragEnd - sp.localOff),
				partial: fragStart > matchStart || fragEnd < matchEnd,
				font:    sp.font,
				size:    sp.size,
			})
		}

		// Продолжаем поиск со следующего символа после найденного.
		i = matchEnd

		key := strings.TrimSpace(combined[bodyStart:bodyEnd])

		// Один ран — within-run, пропускаем (обрабатывается segOfRun).
		if len(contributing) <= 1 {
			continue
		}

		// Согласованность размеров: раны плейсхолдера должны иметь одинаковый
		// размер шрифта, иначе значение выйдет разнокалиберным. Различия в
		// ШРИФТАХ допустимы: значение рендерится в первый (левый) фрагмент,
		// остальные схлопываются в пустые строки с компенсацией ширины, а при
		// отсутствии покрытия шрифтом первого фрагмента файловый слой
		// переключает ран на внедрённый шрифт через провайдер кодеков.
		refSize := contributing[0].size
		skip := false
		for _, c := range contributing[1:] {
			if c.size != refSize {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		// Суммарная ширина исходных фрагментов.
		var origWidth float64
		for _, c := range contributing {
			origWidth += textWidth(c.text)
		}

		val, _ := values[key]
		result = append(result, &crossRunField{
			key:       key,
			value:     val,
			parts:     contributing,
			origWidth: origWidth,
		})
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// replaceCrossRunFields планирует правки для межрановых плейсхолдеров:
// значение рендерится в первый (левый) фрагмент, остальные фрагменты
// схлопываются в пустые строки с кернинговой компенсацией ширины.
// Ран с несколькими заменами пропускается (редкий конфликт вёрстки).
// toks — токены исходного потока (для переименования Tf первого фрагмента
// при переключении на внедрённый шрифт); nil, если переключение не нужно.
// Размер шрифта значения сохраняется исходным (как у плейсхолдера) —
// автоусадка под ширину бокса не выполняется.
func replaceCrossRunFields(runs []streamRun, crossFields []*crossRunField, allowWider bool, provider CodecProvider, toks []token) ([]editRange, int, int) {
	if len(crossFields) == 0 {
		return nil, 0, 0
	}

	// Считаем, сколько замен попадает в каждый ран.
	perRunCount := make([]int, len(runs))
	for _, cf := range crossFields {
		if cf.value == "" || !crossWider(cf, allowWider, provider) {
			continue
		}
		for _, part := range cf.parts {
			if part.run < len(runs) {
				perRunCount[part.run]++
			}
		}
	}

	var edits []editRange
	replaced := 0
	skipped := 0

	for _, cf := range crossFields {
		if cf.value == "" {
			skipped++
			continue
		}

		// Кодек переключения первого фрагмента на внедрённый шрифт.
		var codec RunCodec
		if provider != nil {
			codec = provider(cf.parts[0].font, cf.value)
		}
		if !allowWider && dstWidth(codec, cf.value) > cf.origWidth {
			skipped++
			continue
		}

		// Частичный первый фрагмент: в ране вместе с фрагментом сидит статичный
		// текст (Пример: «Съешь ещё этих » + {{. в одном TJ Word-абзаца). Такой
		// ран заменять целиком нельзя — пропадёт статика. Поле обрабатываем по
		// особой схеме: префикс/суффикс рана сохраняются verbatim, значение
		// рисуется в точке начала фрагмента, остальные фрагменты схлопываются в
		// пустые advance-ноль строки, а относительные Td между ними не трогаются
		// (они задают позиции последующего текста строки и уже учтены геометрией
		// дельты; нейтрализация сдвинула бы хвост строки на фиксированной
		// паре точек влево).
		f0 := cf.parts[0]
		r0 := runs[f0.run]
		lo0 := f0.start - r0.start
		hi0 := lo0 + len(f0.text)
		partial := lo0 != 0 || hi0 != len(r0.text)

		// Кернинговая компенсация ширины — при реальном переключении на
		// внедрённый шрифт (FontName != "") и при эвристическом пути без
		// кодека. Исходный шрифт (originalCodec, FontName == "") рисуется
		// естественной шириной без компенсации.
		var delta float64
		if codec == nil || codec.FontName() != "" {
			delta = cf.origWidth - dstWidth(codec, cf.value)
		}

		first := true
		splitFailed := false
		for _, part := range cf.parts {
			// Ран с несколькими заменами пропускаем (редкий конфликт).
			if perRunCount[part.run] > 1 {
				continue
			}
			r := runs[part.run]
			var newOp string
			if first {
				if partial && toks != nil {
					// Статичный префикс/суффикс рана не теряем: операнд рана
					// режется по границам фрагмента, значение рисуется своей
					// операторной последовательностью, исходный Tf после неё
					// восстанавливается (переименование Tf рана здесь не нужно).
					lo := part.start - r.start
					hi := lo + len(part.text)
					sp := splitRunOperand(toks, r, lo, hi)
					if !sp.ok {
						// Надёжно сопоставить границы фрагмента с токенами не
						// вышло — пропускаем поле целиком: безопаснее оставить
						// плейсхолдер, чем превратить строку в мусор.
						splitFailed = true
						first = false
						break
					}
					newOp = composePartialRun(r, sp, codec, cf.value, delta, part.size)
				} else {
					newOp = renderOperand(cf.value, delta, codec)
					if codec != nil && codec.FontName() != "" {
						// Первый фрагмент рисуется внедрённым шрифтом — Tf рана
						// переименовываем (/F1 → /TF1), иначе CID-байты hex-литерала
						// интерпретируются исходным (некомпозитным) шрифтом.
						if st, en, ok := fontNameRange(toks, r); ok {
							edits = append(edits, editRange{start: st, end: en, to: codec.FontName()})
						}
						// Значение занимает в поле всю ширину плейсхолдера (межрановый
						// случай: каждый символ в своей клетке) — расширяем поле первого
						// фрагмента, чтобы буквы не налезали и не обрезались.
						advPt := dstWidth(codec, cf.value) * part.size / 1000
						edits = append(edits, planFieldWiden(toks, r, advPt)...)
					}
				}
				first = false
			} else if partial {
				// Фрагмент без значения пустится пустой строкой с нулевым
				// продвижением: вся ширина плейсхолдера уже заложена в дельту
				// первого фрагмента. Обязательно TJ-массивом — отдельная "() Tj"
				// оставила бы хвостовое число на стеке операндов, и его подхватил
				// бы следующий оператор (например Tm), ломая вёрстку.
				newOp = "[() 0] TJ"
			} else {
				// Полнорановый межрановый плейсхолдер: фрагмент заменяется пустой
				// строкой с кернинговой компенсацией ширины (фрагменты сидят в
				// отдельных клетках с абсолютным Tm, компенсация возвращает
				// курсор на ширину схлопнутого текста).
				fragWidth := srcWidth(codec, part.text)
				newOp = fmt.Sprintf("[() %g] TJ", -fragWidth)
			}
			edits = append(edits, editRange{start: r.start, end: r.end, to: newOp})
		}
		if splitFailed {
			skipped++
			continue
		}
		// Доводка линии: центрирование широкого значения и обнуление пробелов,
		// перекрывающих внедрённый текст (иначе выглядит, как сбитый кернинг).
		edits = append(edits, crossRunLineFit(runs, toks, cf, codec)...)
		replaced++
	}

	return edits, replaced, skipped
}

// crossWider проверяет допустимость межрановой замены по ширине: при
// allowWider=false значение не должно быть шире области плейсхолдера.
func crossWider(cf *crossRunField, allowWider bool, provider CodecProvider) bool {
	if allowWider {
		return true
	}
	var codec RunCodec
	if provider != nil {
		codec = provider(cf.parts[0].font, cf.value)
	}
	return dstWidth(codec, cf.value) <= cf.origWidth
}

// fragmentSplit — результат разрезания операнда показа рана на статичные
// префикс/суффикс вокруг фрагмента плейсхолдера. Токены фрагмента в результат
// не попадают: они заменяются значением. ok=false — надёжно сопоставить границы
// фрагмента (lo..hi в декодированном r.text) с токенами операнда не удалось
// (произвольное соответствие CID→rune, а не 1:1).
type fragmentSplit struct {
	prefix string // готовый оператор префикса "[...] TJ"; пусто — фрагмент в начале рана
	suffix string // готовый оператор суффикса "[...] TJ"; пусто — фрагмент в конце рана
	ok     bool
}

// splitRunOperand режет операнд показа рана r по фрагменту плейсхолдера
// (lo, hi — байтовые смещения в декодированном r.text). Работает на токенах
// Tj/TJ: литералы массива соответствуют символам r.text в порядке показа, а
// числа-кернинги привязаны к предшествующему литералу. Сопоставление строится
// на числе байтов на символ (2 для композитных CID-шрифтов, 1 для простых),
// которое сверяется с полной длиной литералов операнда — при расхождении (или
// отсутствии токенов) возвращается ok=false и поле пропускается целиком.
func splitRunOperand(toks []token, r streamRun, lo, hi int) fragmentSplit {
	loR := utf8.RuneCountInString(r.text[:lo])
	hiR := utf8.RuneCountInString(r.text[:hi])

	first, last := operandLiteralSpan(toks, r)
	if first < 0 {
		return fragmentSplit{}
	}
	var total int
	for p := first; p <= last; p++ {
		if k := toks[p].kind; k == tokHex || k == tokStr {
			total += len(toks[p].text)
		}
	}
	n := utf8.RuneCountInString(r.text)
	bpc := 0
	if total == n*2 {
		bpc = 2
	} else if total == n {
		bpc = 1
	}
	if bpc == 0 {
		return fragmentSplit{}
	}

	// Границы фрагмента в байтовом пространстве литералов операнда.
	loB, hiB := loR*bpc, hiR*bpc

	var pre, suf strings.Builder
	rawPos := 0   // накопленные байты литералов слева направо
	prevZone := 0 // зона последнего литерала: 1 — префикс, 2 — суффикс, 0 — фрагмент
	for p := first; p <= last; p++ {
		tk := toks[p]
		switch tk.kind {
		case tokNum:
			// Число-кернинг позиционирует следующий за литералом глиф: оно
			// остаётся в той же зоне, что и предшествующий литерал. Граничный
			// кернинг перед фрагментом попадает в префикс — значение рисуется
			// ровно с той точки, где в оригинале стоял первый глиф плейсхолдера.
			switch prevZone {
			case 1:
				pre.WriteByte(' ')
				pre.WriteString(tk.text)
			case 2:
				suf.WriteByte(' ')
				suf.WriteString(tk.text)
			}
		case tokHex, tokStr:
			n := len(tk.text)
			start := rawPos
			end := start + n
			rawPos = end
			zone := 0
			if start < loB {
				cut := end
				if cut > loB {
					cut = loB
				}
				writeLiteral(&pre, tk, 0, cut-start)
				zone = 1
			}
			if end > hiB {
				cut := start
				if cut < hiB {
					cut = hiB
				}
				writeLiteral(&suf, tk, cut-start, end-start)
				zone = 2
			}
			prevZone = zone
		}
	}

	sp := fragmentSplit{ok: true}
	if strings.TrimSpace(pre.String()) != "" {
		sp.prefix = "[ " + strings.TrimSpace(pre.String()) + " ] TJ"
	}
	if strings.TrimSpace(suf.String()) != "" {
		sp.suffix = "[ " + strings.TrimSpace(suf.String()) + " ] TJ"
	}
	return sp
}

// operandLiteralSpan возвращает диапазон индексов токенов операнда показа рана
// ([+1..-1] для TJ — включая числа-кернинги, единственный литерал для Tj).
// Отрицательный first — операнд не распознан.
func operandLiteralSpan(toks []token, r streamRun) (int, int) {
	if r.opType == "Tj" {
		p := r.opTok - 1 // литерал Tj: ((...)) либо <...>
		if p >= 0 && (toks[p].kind == tokHex || toks[p].kind == tokStr) {
			return p, p
		}
		return -1, -1
	}
	for open := r.opTok - 1; open >= 0; open-- {
		if toks[open].kind == tokOpen {
			return open + 1, r.opTok - 1
		}
	}
	return -1, -1
}

// writeLiteral записывает байтовый поддиапазон [lo,hi) литерала токена в
// потоковой форме (гекс-литерал <...> либо строка (...) с экранированием).
func writeLiteral(sb *strings.Builder, tk token, lo, hi int) {
	sb.WriteByte(' ')
	body := tk.text[lo:hi]
	switch tk.kind {
	case tokHex:
		sb.WriteString("<" + hexEncode([]byte(body)) + ">")
	default:
		sb.WriteString("(" + quoteString(body) + ")")
	}
}

// composePartialRun собирает операторную последовательность для рана с
// частичным фрагментом: статичный префикс/суффикс (в исходном шрифте raра),
// значение в точке фрагмента с переключением на внедрённый шрифт (если нужен
// FontName) и восстановлением исходного Tf после значения.
func composePartialRun(r streamRun, sp fragmentSplit, codec RunCodec, value string, delta, size float64) string {
	var sb strings.Builder
	if sp.prefix != "" {
		sb.WriteString(sp.prefix)
		sb.WriteByte(' ')
	}
	needSwitch := codec != nil && codec.FontName() != ""
	if needSwitch {
		sb.WriteString(codec.FontName())
		sb.WriteByte(' ')
		sb.WriteString(fmtNum(size))
		sb.WriteString(" Tf ")
	}
	sb.WriteString(renderOperand(value, delta, codec))
	if needSwitch {
		sb.WriteByte(' ')
		sb.WriteString(r.font)
		sb.WriteByte(' ')
		sb.WriteString(fmtNum(size))
		sb.WriteString(" Tf")
	}
	if sp.suffix != "" {
		sb.WriteByte(' ')
		sb.WriteString(sp.suffix)
	}
	return sb.String()
}

// applyEdits применяет правки к потоку справа налево (по убыванию start):
// расширение/сжатие ранних фрагментов не сдвигает позиции поздних.
func applyEdits(s string, edits []editRange) string {
	if len(edits) == 0 {
		return s
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	for _, e := range edits {
		s = padReplace(s, e.start, e.end, e.to)
	}
	return s
}

// ReplaceParams — параметры замены плейсхолдеров в контентном потоке PDF.
// Аналог WconvPipeline из pkg/convert, но для pdfpl.
type ReplaceParams struct {
	Stream string            // содержимое контентного потока
	Values map[string]string // ключ → значение (голые ключи, без точки)

	// AllowWider разрешает замену значениями, шире исходного текста
	// плейсхолдера (текст выходит за пределы исходной линии вправо).
	// По умолчанию (false) такие значения пропускаются. Для файлового
	// слоя (ReplaceFile) всегда включён.
	AllowWider bool

	// Codec — необязательный провайдер кодеков для переключения ранов на
	// внедрённые шрифты (файловый слой ReplaceFile). nil — раны остаются в
	// исходных шрифтах, ширина текста считается эвристически.
	Codec CodecProvider

	// Decode — необязательная функция трансляции логического текста рана перед
	// поиском плейсхолдеров. Применяется к рану (font — активный шрифт /F1),
	// чей текст закодирован байтами-символами (композитные Identity-H шрифты с
	// ToUnicode-картой, типично для сабсетов MS Word): hex-CID байты переводятся
	// в Unicode, чтобы findFields увидел {{.Key}}. Для шрифтов, не требующих
	// декодирования, возвращает исходный текст. nil — не декодировать.
	Decode func(font, text string) string
}

// ReplaceResult — результат замены плейсхолдеров в контентном потоке.
type ReplaceResult struct {
	Stream   string // модифицированный контентный поток
	Replaced int    // количество заменённых плейсхолдеров
	Skipped  int    // количество пропущенных (ключ не найден или значение шире)
}

// Replace заменяет плейсхолдеры {{.Key}}/{{Key}} в контентном потоке PDF
// значениями из Values. Замена сохраняет ширину текста: разница в ширине
// компенсируется числом кернинга в TJ-массиве.
//
// Поддерживает как однорановые, так и межрановые (cross-run) плейсхолдеры,
// разнесённые по нескольким BT/ET-блокам (типично для PDF от MS Word).
// Плейсхолдер, ключ которого отсутствует в Values или значение которого шире
// исходного текста, остаётся не тронутым и учитывается в Skipped.
//
// Формат результата совместим с конвенциями wconv: чистая функция без
// побочных эффектов, принимает параметры структурно и возвращает результат.
func Replace(params *ReplaceParams) (*ReplaceResult, error) {
	if params == nil || params.Stream == "" {
		return &ReplaceResult{Stream: params.Stream, Replaced: 0, Skipped: 0}, nil
	}

	runs, err := textRuns(params.Stream)
	if err != nil {
		return nil, fmt.Errorf("разбор контентного потока: %w", err)
	}

	// Трансляция логического текста ранов (hex-CID → Unicode для Identity-H
	// сабсетов) до поиска полей, чтобы {{.Key}} были видны findFields.
	if params.Decode != nil {
		for i := range runs {
			runs[i].text = params.Decode(runs[i].font, runs[i].text)
		}
	}

	fields := findFields(params.Stream, runs)

	_, replaced, skipped, edits, err := buildStreamCounted(params.Stream, runs, fields, params.Values, params.AllowWider, params.Codec)
	if err != nil {
		return nil, err
	}

	return &ReplaceResult{
		Stream:   applyEdits(params.Stream, edits),
		Replaced: replaced,
		Skipped:  skipped,
	}, nil
}

// ListPlaceholders возвращает имена всех плейсхолдеров {{.Key}}/{{Key}},
// найденных в контентном потоке: и однорановых, и межрановых (cross-run).
// Полезно для диагностики/сухого прогона: какие ключи шаблон ждёт из данных.
func ListPlaceholders(stream string) ([]string, error) {
	runs, err := textRuns(stream)
	if err != nil {
		return nil, fmt.Errorf("разбор контентного потока: %w", err)
	}

	seen := make(map[string]bool)
	for _, f := range findFields(stream, runs) {
		seen[f.key] = true
	}
	for _, cf := range buildCrossRunFields(runs, map[string]string{}) {
		seen[cf.key] = true
	}

	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}
