package pdfpl

import (
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"
)

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

	combined, spans := buildRunSpans(runs)
	var result []*crossRunField

	// Проходим по плейсхолдерам, отслеживая вхождение в range-блоки:
	// {{range ...}}...{{end}} игнорируются целиком.
	inRange := false
	i := 0
	for i < len(combined) {
		if !inRange {
			if after, ok := crossRangeOpen(combined, i); ok {
				inRange = true
				i = after
				continue
			}
		} else {
			after, closed, cont := advanceCrossRange(combined, i)
			if !cont {
				break
			}
			i = after
			inRange = !closed
			continue
		}

		m := placeholderRe.FindStringSubmatchIndex(combined[i:])
		if m == nil {
			break
		}
		matchStart, matchEnd := i+m[0], i+m[1]
		if cf := buildCrossField(spans, combined, matchStart, matchEnd, i+m[2], i+m[3], values); cf != nil {
			result = append(result, cf)
		}
		// Продолжаем поиск со следующего символа после найденного.
		i = matchEnd
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// runSpan — вкладка одного рана в склеенную строку всех ранов страницы.
type runSpan struct {
	run      int
	localOff int // смещение начала рана в combined
	localEnd int // смещение конца рана в combined (исключ.)
	absStart int // абсолютное смещение в потоке
	font     string
	size     float64
}

// buildRunSpans склеивает тексты ранов в единую строку и строит карту span'ов
// (символ склейки -> ран).
func buildRunSpans(runs []streamRun) (string, []runSpan) {
	var full strings.Builder
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
	return full.String(), spans
}

// crossRangeOpen возвращает позицию сразу после открывающего {{range}}
// плейсхолдера, если он виден от i; ok=false — открытия нет.
func crossRangeOpen(combined string, i int) (int, bool) {
	rel := strings.Index(combined[i:], "{{range")
	if rel < 0 {
		return 0, false
	}
	idx := i + rel
	ri := placeholderRe.FindStringIndex(combined[idx:])
	if ri == nil || !strings.HasPrefix(strings.TrimSpace(combined[idx+ri[0]:idx+ri[1]]), "{{range") {
		return 0, false
	}
	return idx + ri[1], true
}

// advanceCrossRange двигается по range-блоку: закрывает его (closed=true) либо
// перескакивает к следующему плейсхолдеру, чтобы не выйти за range.
// cont=false — выйти из сканера.
func advanceCrossRange(combined string, i int) (after int, closed, cont bool) {
	if after, ok := skipRangeBlock(combined, i); ok {
		return after, true, true
	}
	if nxt := placeholderRe.FindStringIndex(combined[i:]); nxt != nil {
		return i + nxt[0], false, true
	}
	return 0, false, false
}

// buildCrossField собирает crossRunField для найденного плейсхолдера; nil, если
// поле не подходит (один ран — обрабатывается within-run, либо рассогласованные
// размеры шрифтов фрагментов).
func buildCrossField(spans []runSpan, combined string, matchStart, matchEnd, bodyStart, bodyEnd int, values map[string]string) *crossRunField {
	var contributing []crossRunContrib
	for _, sp := range spans {
		fragStart, fragEnd := matchStart, matchEnd
		if fragStart < sp.localOff {
			fragStart = sp.localOff
		}
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

	// Один ран — within-run, пропускаем (обрабатывается segOfRun).
	if len(contributing) <= 1 {
		return nil
	}

	// Согласованность размеров: у фрагментов поля должен быть одинаковый
	// размер шрифта, иначе значение выйдет разнокалиберным.
	refSize := contributing[0].size
	for _, c := range contributing[1:] {
		if c.size != refSize {
			return nil
		}
	}

	// Суммарная ширина исходных фрагментов.
	var origWidth float64
	for _, c := range contributing {
		origWidth += textWidth(c.text)
	}

	key := strings.TrimSpace(combined[bodyStart:bodyEnd])
	val, _ := values[key]
	return &crossRunField{key: key, value: val, parts: contributing, origWidth: origWidth}
}

func replaceCrossRunFields(runs []streamRun, crossFields []*crossRunField, allowWider bool, provider CodecProvider, toks []token) ([]editRange, int, int) {
	if len(crossFields) == 0 {
		return nil, 0, 0
	}

	// Считаем, сколько замен попадает в каждый ран.
	perRunCount := countCrossPerRun(runs, crossFields, allowWider, provider)

	var edits []editRange
	replaced := 0
	skipped := 0

	for _, cf := range crossFields {
		if cf.value == "" {
			skipped++
			continue
		}

		// Кодек переключения первого фрагмента на внедрённый шрифт.
		codec := crossCodec(provider, cf)
		if !allowWider && dstWidth(codec, cf.value) > cf.origWidth {
			skipped++
			continue
		}

		partial, delta := crossGeom(runs, cf, codec)
		baseEditsLen := len(edits)
		cellEdits, ok := crossFieldEdits(runs, toks, cf, codec, partial, delta, perRunCount)
		if !ok {
			// Фрагмент не сопоставился с токенами — откатываем правки поля
			// целиком, чтобы частичная замена не осталась в потоке.
			edits = edits[:baseEditsLen]
			skipped++
			continue
		}
		edits = append(edits, cellEdits...)
		// Доводка линии: центрирование широкого значения и обнуление
		// пробелов, перекрывающих внедрённый текст.
		edits = append(edits, crossRunLineFit(runs, toks, cf, codec)...)
		replaced++
	}

	return edits, replaced, skipped
}

// countCrossPerRun считает, сколько замен попадает в каждый ран (раны с
// несколькими полями пропускаются — редкий конфликт вёрстки).
func countCrossPerRun(runs []streamRun, crossFields []*crossRunField, allowWider bool, provider CodecProvider) []int {
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
	return perRunCount
}

// crossCodec поставляет кодек внедрённого шрифта для первого фрагмента поля.
func crossCodec(provider CodecProvider, cf *crossRunField) RunCodec {
	if provider == nil {
		return nil
	}
	return provider(cf.parts[0].font, cf.value)
}

// crossGeom определяет, частичный ли первый фрагмент (рядом со статикой), и
// кернинговую дельту ширины значения относительно исходной ширины поля.
func crossGeom(runs []streamRun, cf *crossRunField, codec RunCodec) (partial bool, delta float64) {
	f0 := cf.parts[0]
	r0 := runs[f0.run]
	lo0 := f0.start - r0.start
	hi0 := lo0 + len(f0.text)
	partial = lo0 != 0 || hi0 != len(r0.text)
	// Исходный шрифт (originalCodec, FontName == "") рисуется естественной
	// шириной без компенсации.
	if codec == nil || codec.FontName() != "" {
		delta = cf.origWidth - dstWidth(codec, cf.value)
	}
	return partial, delta
}

// crossFirstFragment строит оператор и сопутствующие правки для первого
// (левого) фрагмента поля: значение в рендере внедрённым шрифтом с
// переименованием Tf и расширением клетки либо полная пересборка рана с
// частичным фрагментом (composePartialRun) через splitRunOperand.
func crossFirstFragment(runs []streamRun, toks []token, cf *crossRunField, part crossRunContrib, partial bool, codec RunCodec, delta float64) (newOp string, extra []editRange, ok bool) {
	r := runs[part.run]
	if partial && toks != nil {
		lo := part.start - r.start
		hi := lo + len(part.text)
		sp := splitRunOperand(toks, r, lo, hi)
		if !sp.ok {
			return "", nil, false
		}
		return composePartialRun(r, sp, codec, cf.value, delta, part.size), nil, true
	}

	newOp = renderOperand(cf.value, delta, codec)
	if codec == nil || codec.FontName() == "" {
		return newOp, nil, true
	}

	// Первый фрагмент рисуется внедрённым шрифтом — Tf рана переименовываем
	// (/F1 -> /TF1), иначе CID-байты hex-литерала интерпретируются исходным
	// (некомпозитным) шрифтом.
	if st, en, ok := fontNameRange(toks, r); ok {
		extra = append(extra, editRange{start: st, end: en, to: codec.FontName()})
	}
	// Значение занимает в поле всю ширину плейсхолдера (межрановый случай) —
	// расширяем клетку первого фрагмента, чтобы буквы не налезли и не обрезались.
	advPt := dstWidth(codec, cf.value) * part.size / 1000
	extra = append(extra, planFieldWiden(toks, r, advPt)...)
	return newOp, extra, true
}

// crossTrailingFragment строит оператор для не-первого фрагмента поля: пустая
// строка с кернинговой компенсацией ширины (полный фрагмент в отдельной клетке)
// либо сохранение статики вокруг частичного фрагмента.
func crossTrailingFragment(toks []token, r streamRun, part crossRunContrib, partial bool, codec RunCodec) (string, bool) {
	if !partial {
		return fmt.Sprintf("[() %g] TJ", -srcWidth(codec, part.text)), true
	}
	if toks == nil {
		return "[() 0] TJ", true
	}
	lo := part.start - r.start
	hi := lo + len(part.text)
	sp := splitRunOperand(toks, r, lo, hi)
	if !sp.ok {
		return "", false
	}
	switch {
	case sp.prefix == "" && sp.suffix == "":
		return "[() 0] TJ", true
	case sp.prefix == "":
		return sp.suffix, true
	case sp.suffix == "":
		return sp.prefix, true
	default:
		return sp.prefix + " " + sp.suffix, true
	}
}

// crossFieldEdits планирует правки всех клеток одного поля. ok=false — фрагмент
// не удалось надёжно сопоставить токенам, поле пропускается целиком.
func crossFieldEdits(runs []streamRun, toks []token, cf *crossRunField, codec RunCodec, partial bool, delta float64, perRunCount []int) ([]editRange, bool) {
	var dets []editRange
	first := true
	for _, part := range cf.parts {
		// Ран с несколькими заменами пропускаем (редкий конфликт вёрстки).
		if perRunCount[part.run] > 1 {
			continue
		}
		r := runs[part.run]
		var newOp string
		var ok bool
		if first {
			var extra []editRange
			newOp, extra, ok = crossFirstFragment(runs, toks, cf, part, partial, codec, delta)
			if ok {
				dets = append(dets, extra...)
			}
			first = false
		} else {
			newOp, ok = crossTrailingFragment(toks, r, part, partial, codec)
		}
		if !ok {
			return nil, false
		}
		dets = append(dets, editRange{start: r.start, end: r.end, to: newOp})
	}
	return dets, true
}

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
	bpc := crossBpc(toks, first, last, utf8.RuneCountInString(r.text))
	if bpc == 0 {
		slog.Warn("pdfpl: длина литералов операнда не согласуется с текстом рана", slog.String("font", r.font), slog.String("op", r.opType))
		return fragmentSplit{}
	}

	// Границы фрагмента в байтовом пространстве литералов операнда.
	loB, hiB := loR*bpc, hiR*bpc

	var pre, suf strings.Builder
	splitLiteralZones(toks, first, last, loB, hiB, &pre, &suf)

	sp := fragmentSplit{ok: true}
	if strings.TrimSpace(pre.String()) != "" {
		sp.prefix = "[ " + strings.TrimSpace(pre.String()) + " ] TJ"
	}
	if strings.TrimSpace(suf.String()) != "" {
		sp.suffix = "[ " + strings.TrimSpace(suf.String()) + " ] TJ"
	}
	return sp
}

// crossBpc определяет число байт литерала на символ r.text (1 или 2) по общей
// длине литералов операнда; 0 — если согласовать с длиной текста не удалось.
func crossBpc(toks []token, first, last, n int) int {
	var total int
	for p := first; p <= last; p++ {
		if k := toks[p].kind; k == tokHex || k == tokStr {
			total += len(toks[p].text)
		}
	}
	if total == n*2 {
		return 2
	}
	if total == n {
		return 1
	}
	return 0
}

// splitLiteralZones раскладывает литералы операнда по зонам относительно окна
// фрагмента [loB, hiB): префикс (pre) и суффикс (suf) накапливаются с
// сохранением чисел-кернингов, токены самого фрагмента отбрасываются.
func splitLiteralZones(toks []token, first, last, loB, hiB int, pre, suf *strings.Builder) {
	rawPos := 0 // накопленные байты литералов слева направо
	prevZone := 0
	for p := first; p <= last; p++ {
		tk := toks[p]
		switch tk.kind {
		case tokNum:
			// Число-кернинг позиционирует следующий глиф — остаётся в той же
			// зоне, что и предыдущий литерал.
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
				writeLiteral(pre, tk, 0, cut-start)
				zone = 1
			}
			if end > hiB {
				cut := start
				if cut < hiB {
					cut = hiB
				}
				writeLiteral(suf, tk, cut-start, end-start)
				zone = 2
			}
			prevZone = zone
		}
	}
}

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
