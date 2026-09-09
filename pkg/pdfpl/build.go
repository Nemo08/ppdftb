package pdfpl

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
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
	for p := r.opTok - 1; p >= 0; p-- {
		if toks[p].kind != tokOp {
			continue
		}
		switch toks[p].text {
		case "Tm", "Td", "TD":
			if p < 2 || toks[p-1].kind != tokNum || toks[p-2].kind != tokNum {
				return 0, false
			}
			x, err := strconv.ParseFloat(toks[p-2].text, 64)
			if err != nil {
				return 0, false
			}
			return x, true
		}
	}
	return 0, false
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
func buildCrossRunFields(runs []streamRun, values map[string]string) map[string]*crossRunField {
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
	result := make(map[string]*crossRunField)

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

		// Согласованность шрифтов: если шрифты/размеры различаются,
		// пропускаем весь плейсхолдер (небезопасно заменять).
		refFont := contributing[0].font
		refSize := contributing[0].size
		skip := false
		for _, c := range contributing[1:] {
			if c.font != refFont || c.size != refSize {
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
		result[key] = &crossRunField{
			key:       key,
			value:     val,
			parts:     contributing,
			origWidth: origWidth,
		}
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
func replaceCrossRunFields(runs []streamRun, crossFields map[string]*crossRunField, allowWider bool, provider CodecProvider, toks []token) ([]editRange, int, int) {
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

		first := true
		for _, part := range cf.parts {
			// Ран с несколькими заменами пропускаем (редкий конфликт).
			if perRunCount[part.run] > 1 {
				continue
			}
			r := runs[part.run]
			var newOp string
			if first {
				// Кернинговая компенсация ширины — при реальном переключении на
				// внедрённый шрифт (FontName != "") и при эвристическом пути без
				// кодека. Исходный шрифт (originalCodec, FontName == "") рисуется
				// естественной шириной без компенсации.
				var delta float64
				if codec == nil || codec.FontName() != "" {
					delta = cf.origWidth - dstWidth(codec, cf.value)
				}
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
				first = false
			} else {
				// Фрагмент плейсхолдера, не содержащий значения, заменяется пустой
				// строкой с кернинговой компенсацией ширины. Обязательно TJ-массивом:
				// отдельная "() -N Tj" оставила бы число -N на стеке операндов, и его
				// подхватил бы следующий оператор (например Tm), ломая всю вёрстку.
				fragWidth := srcWidth(codec, part.text)
				newOp = fmt.Sprintf("[() %g] TJ", -fragWidth)
			}
			edits = append(edits, editRange{start: r.start, end: r.end, to: newOp})
		}
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
	for k := range buildCrossRunFields(runs, map[string]string{}) {
		seen[k] = true
	}

	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}
