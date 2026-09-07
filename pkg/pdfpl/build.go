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
	modified, _, _, err := buildStreamCounted(stream, runs, fields, values, false)
	return modified, err
}

// buildStreamCounted — то же, что buildStream, но дополнительно возвращает
// количество заменённых и пропущенных плейсхолдеров.
//
// allowWider=true разрешает замену значениями, шире исходного текста
// плейсхолдера (текст «выедет» вправо, за пределы исходной линии). При
// allowWider=false такие значения пропускаются (безопасное поведение по умолчанию).
func buildStreamCounted(stream string, runs []streamRun, fields []field, values map[string]string, allowWider bool) (string, int, int, error) {
	// Межрановые плейсхолдеры заменяются на уровне ран — раны, участвующие
	// в них, исключаем из однорановой обработки. Межрановые плейсхолдеры
	// обрабатываем ВСЕГДА (даже когда fields пуст, т.к. findFields не видит
	// фрагменты {{. разнесённые по ранам).
	crossFields := buildCrossRunFields(runs, values)
	modified, crossReplaced, crossSkipped := replaceCrossRunFields(stream, runs, crossFields, allowWider)

	if len(fields) == 0 {
		return modified, crossReplaced, crossSkipped, nil
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
	for i, r := range runs {
		fl := byRun[i]
		if len(fl) == 0 {
			continue
		}
		newOperand, err := rebuildOperand(r, fl, values, allowWider)
		if err != nil {
			return "", 0, 0, err
		}
		if newOperand == "" {
			crossSkipped += len(fl)
			continue
		}
		withinReplaced += len(fl)
		modified = padReplace(modified, r.start, r.end, newOperand)
	}
	return modified, crossReplaced + withinReplaced, crossSkipped, nil
}

// padReplace заменяет срез [lo, hi) строки s на replacement.
func padReplace(s string, lo, hi int, replacement string) string {
	return s[:lo] + replacement + s[hi:]
}

// rebuildOperand пересобирает операнд ран-блока, заменяя поля значениями.
// Возвращает пустую строку, если замена не произошла (ключ отсутствует в values,
// или — при allowWider=false — значение шире плейсхолдера).
// Операнд рендерится как TJ-массив, если потребовалась компенсация ширины;
// иначе сохраняется строковая форма (... ) Tj.
func rebuildOperand(r streamRun, fields []field, values map[string]string, allowWider bool) (string, error) {
	segs := segOfRun(r, fields)
	if len(segs) == 0 {
		return "", nil
	}

	// Проверяем наличие всех значений и суммарную дельту ширины.
	var sb strings.Builder
	var delta float64 // сумма (ширина плейсхолдера - ширина значения)
	for _, s := range segs {
		if s.kind == segText {
			sb.WriteString(s.text)
			continue
		}
		val, ok := values[s.value]
		if !ok {
			return "", nil
		}
		// Значение шире плейсхолдера — замену отменяем целиком,
		// если только явно не разрешено (allowWider).
		if !allowWider && textWidth(val) > textWidth(s.text) {
			return "", nil
		}
		delta += textWidth(s.text) - textWidth(val)
		sb.WriteString(val)
	}

	quoted := quoteString(sb.String())
	// Дельта строго положительна: нужно компенсировать смещение пера
	// хвостовым числом кернинга в TJ-массиве.
	if delta > 0 {
		return fmt.Sprintf("[%s %g] TJ", "("+quoted+")", -delta), nil
	}
	return "(" + quoted + ") Tj", nil
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

// replaceCrossRunFields заменяет межрановые плейсхолдеры в потоке.
// Возвращает модифицированный поток, количество замен и пропусков.
//
// Алгоритм замены для каждого crossRunField:
//   - Первый участвующий ран: значение + кернинг -(origWidth - textWidth(value))
//   - Последующие участвующие раны: пустой операнд () + кернинг -(fragmentWidth)
//
// Кернинг суммарно компенсирует разницу ширин, сохраняя позицию пера.
//
// Ограничение: в целевых PDF (MS Word) каждый фрагмент {{ или }} занимает
// отдельный ран, поэтому каждый ран получает ровно одну замену. Если один ран
// содержит фрагменты двух разных плейсхолдеров (редкий случай), он пропускается,
// чтобы не повредить соседний операнд.
//
// allowWider=true разрешает значения шире исходного текста плейсхолдера.
func replaceCrossRunFields(stream string, runs []streamRun, crossFields map[string]*crossRunField, allowWider bool) (string, int, int) {
	if len(crossFields) == 0 {
		return stream, 0, 0
	}

	// Считаем, сколько замен попадает в каждый ран.
	perRunCount := make([]int, len(runs))
	for _, cf := range crossFields {
		if cf.value == "" || (!allowWider && textWidth(cf.value) > cf.origWidth) {
			continue
		}
		for _, part := range cf.parts {
			if part.run < len(runs) {
				perRunCount[part.run]++
			}
		}
	}

	// Собираем замены: для каждого рана — список (start, end, newOperand).
	type replacement struct {
		start, end int
		newOp      string
	}
	byRun := make(map[int][]replacement)
	replaced := 0
	skipped := 0

	for _, cf := range crossFields {
		if cf.value == "" {
			skipped++
			continue
		}
		if !allowWider && textWidth(cf.value) > cf.origWidth {
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
				val := quoteString(cf.value)
				delta := cf.origWidth - textWidth(cf.value)
				if delta > 0 {
					newOp = fmt.Sprintf("[%s %g] TJ", "("+val+")", -delta)
				} else {
					newOp = "(" + val + ") Tj"
				}
				first = false
			} else {
				fragWidth := textWidth(part.text)
				newOp = fmt.Sprintf("() %g Tj", -fragWidth)
			}
			byRun[part.run] = append(byRun[part.run], replacement{
				start: r.start,
				end:   r.end,
				newOp: newOp,
			})
		}
		replaced++
	}

	// Применяем замены глобально справа налево (по убыванию start), чтобы
	// расширение/сжатие ранних (левых) ран не сдвигало позиции поздних.
	modified := stream
	var all []replacement
	for _, reps := range byRun {
		all = append(all, reps...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].start > all[j].start })
	for _, rep := range all {
		modified = padReplace(modified, rep.start, rep.end, rep.newOp)
	}

	return modified, replaced, skipped
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

	modified, replaced, skipped, err := buildStreamCounted(params.Stream, runs, fields, params.Values, params.AllowWider)
	if err != nil {
		return nil, err
	}

	return &ReplaceResult{Stream: modified, Replaced: replaced, Skipped: skipped}, nil
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
