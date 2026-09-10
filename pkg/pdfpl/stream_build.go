package pdfpl

import (
	"fmt"
	"sort"
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
	// в них, исключаем из однорановой обработки (findFields их не видит).
	crossFields := buildCrossRunFields(runs, values)
	crossEdits, crossReplaced, crossSkipped := replaceCrossRunFields(runs, crossFields, allowWider, provider, toks)
	edits = append(edits, crossEdits...)

	if len(fields) == 0 {
		return stream, crossReplaced, crossSkipped, edits, nil
	}

	// Множество ранов, уже обработанных межрановой заменой.
	consumed := crossConsumedRuns(crossFields, allowWider)

	// Группируем однорановые поля по ранам — каждый ран пересобирается целиком.
	byRun := runFieldGroups(fields, consumed)

	withinReplaced := 0
	withinSkipped := 0
	for i, r := range runs {
		fl := byRun[i]
		if len(fl) == 0 {
			continue
		}
		var repl, skp int
		edits, repl, skp, err = planWithinRun(runs, i, r, fl, values, allowWider, provider, toks, edits)
		if err != nil {
			return "", 0, 0, nil, err
		}
		withinReplaced += repl
		withinSkipped += skp
	}
	return stream, crossReplaced + withinReplaced, crossSkipped + withinSkipped, edits, nil
}

// crossConsumedRuns собирает раны, уже обработанные межрановой заменой, чтобы
// исключить их из одноранового пересбора. allowWider=false исключает поля
// шире исходного текста (безопасное поведение по умолчанию).
func crossConsumedRuns(crossFields []*crossRunField, allowWider bool) map[int]bool {
	consumed := make(map[int]bool)
	for _, cf := range crossFields {
		if cf.value == "" || (!allowWider && textWidth(cf.value) > cf.origWidth) {
			continue
		}
		for _, part := range cf.parts {
			consumed[part.run] = true
		}
	}
	return consumed
}

// runFieldGroups группирует однорановые поля по индексу рана; раны из
// consumed исключаются из пересбора.
func runFieldGroups(fields []field, consumed map[int]bool) map[int][]field {
	byRun := map[int][]field{}
	for _, f := range fields {
		if consumed[f.run] {
			continue
		}
		byRun[f.run] = append(byRun[f.run], f)
	}
	return byRun
}

// planWithinRun планирует правки однорановой замены: операнд рана,
// переименование Tf на внедрённый шрифт, расширение поля (клетки Word) и
// доводку линии. Возвращает обновлённые правки и число замен/пропусков.
func planWithinRun(runs []streamRun, i int, r streamRun, fl []field, values map[string]string, allowWider bool, provider CodecProvider, toks []token, edits []editRange) ([]editRange, int, int, error) {
	newOperand, codec, advPt, err := rebuildOperand(r, fl, values, allowWider, provider)
	if err != nil {
		return nil, 0, 0, err
	}
	if newOperand == "" {
		return nil, 0, len(fl), nil
	}
	edits = append(edits, editRange{start: r.start, end: r.end, to: newOperand})
	if codec != nil && codec.FontName() != "" {
		// Значение рисуется внедрённым шрифтом — Tf рана переименовываем.
		if st, en, ok := fontNameRange(toks, r); ok {
			edits = append(edits, editRange{start: st, end: en, to: codec.FontName()})
		}
		// Расширяем поле (clip-клетку Word), чтобы длинный текст не обрезался.
		edits = append(edits, planFieldWiden(toks, r, advPt)...)
		// Доводка линии: центрирование и обнуление перекрывающих пробелов.
		edits = append(edits, fitSingleLine(runs, toks, i, r, advPt, codec)...)
	}
	return edits, 1, 0, nil
}

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
