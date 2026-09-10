package pdfpl

import (
	"fmt"
	"sort"
	"strings"
)

// buildStream ╨╖╨░╨╝╨╡╨╜╤П╨╡╤В ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╤Л ╨▓ ╨║╨╛╨╜╤В╨╡╨╜╤В╨╜╨╛╨╝ ╨┐╨╛╤В╨╛╨║╨╡ ╨╖╨╜╨░╤З╨╡╨╜╨╕╤П╨╝╨╕ ╨╕╨╖ values.
// ╨Я╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А, ╨╖╨╜╨░╤З╨╡╨╜╨╕╨╡ ╨║╨╛╤В╨╛╤А╨╛╨│╨╛ ╤И╨╕╤А╨╡ ╨╕╤Б╤Е╨╛╨┤╨╜╨╛╨│╨╛ ╤В╨╡╨║╤Б╤В╨░, ╨╛╤Б╤В╨░╤С╤В╤Б╤П ╨╜╨╡ ╤В╤А╨╛╨╜╤Г╤В╤Л╨╝.
// ╨Ч╨░╨╝╨╡╨╜╨░ ╤Б╨╛╤Е╤А╨░╨╜╤П╨╡╤В ╤И╨╕╤А╨╕╨╜╤Г ╤В╨╡╨║╤Б╤В╨░: ╤А╨░╨╖╨╜╨╕╤Ж╨░ ╨▓ ╤И╨╕╤А╨╕╨╜╨╡ ╨║╨╛╨╝╨┐╨╡╨╜╤Б╨╕╤А╤Г╨╡╤В╤Б╤П ╤З╨╕╤Б╨╗╨╛╨╝
// ╨║╨╡╤А╨╜╨╕╨╜╨│╨░ ╨▓ TJ-╨╝╨░╤Б╤Б╨╕╨▓╨╡, ╤З╤В╨╛╨▒╤Л ╤Б╨╗╨╡╨┤╤Г╤О╤Й╨╕╨╣ ╤В╨╡╨║╤Б╤В ╨╜╨╡ ╨┐╨╛╨╡╤Е╨░╨╗.
//
// ╨Ю╨▒╤А╨░╨▒╨░╤В╤Л╨▓╨░╨╡╤В ╨╕ ╨╛╨┤╨╜╨╛╤А╨░╨╜╨╛╨▓╤Л╨╡, ╨╕ ╨╝╨╡╨╢╤А╨░╨╜╨╛╨▓╤Л╨╡ (cross-run) ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╤Л.
func buildStream(stream string, runs []streamRun, fields []field, values map[string]string) (string, error) {
	_, _, _, edits, err := buildStreamCounted(stream, runs, fields, values, false, nil)
	if err != nil {
		return "", err
	}
	return applyEdits(stream, edits), nil
}

// buildStreamCounted тАФ ╤В╨╛ ╨╢╨╡, ╤З╤В╨╛ buildStream, ╨╜╨╛ ╨┤╨╛╨┐╨╛╨╗╨╜╨╕╤В╨╡╨╗╤М╨╜╨╛ ╨▓╨╛╨╖╨▓╤А╨░╤Й╨░╨╡╤В
// ╨║╨╛╨╗╨╕╤З╨╡╤Б╤В╨▓╨╛ ╨╖╨░╨╝╨╡╨╜╤С╨╜╨╜╤Л╤Е ╨╕ ╨┐╤А╨╛╨┐╤Г╤Й╨╡╨╜╨╜╤Л╤Е ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╨╛╨▓.
//
// allowWider=true ╤А╨░╨╖╤А╨╡╤И╨░╨╡╤В ╨╖╨░╨╝╨╡╨╜╤Г ╨╖╨╜╨░╤З╨╡╨╜╨╕╤П╨╝╨╕, ╤И╨╕╤А╨╡ ╨╕╤Б╤Е╨╛╨┤╨╜╨╛╨│╨╛ ╤В╨╡╨║╤Б╤В╨░
// ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╨░ (╤В╨╡╨║╤Б╤В ┬л╨▓╤Л╨╡╨┤╨╡╤В┬╗ ╨▓╨┐╤А╨░╨▓╨╛, ╨╖╨░ ╨┐╤А╨╡╨┤╨╡╨╗╤Л ╨╕╤Б╤Е╨╛╨┤╨╜╨╛╨╣ ╨╗╨╕╨╜╨╕╨╕). ╨Я╤А╨╕
// allowWider=false ╤В╨░╨║╨╕╨╡ ╨╖╨╜╨░╤З╨╡╨╜╨╕╤П ╨┐╤А╨╛╨┐╤Г╤Б╨║╨░╤О╤В╤Б╤П (╨▒╨╡╨╖╨╛╨┐╨░╤Б╨╜╨╛╨╡ ╨┐╨╛╨▓╨╡╨┤╨╡╨╜╨╕╨╡ ╨┐╨╛ ╤Г╨╝╨╛╨╗╤З╨░╨╜╨╕╤О).
//
// provider тАФ ╨╜╨╡╨╛╨▒╤П╨╖╨░╤В╨╡╨╗╤М╨╜╤Л╨╣ ╨┐╤А╨╛╨▓╨░╨╣╨┤╨╡╤А ╨║╨╛╨┤╨╡╨║╨╛╨▓ ╨┤╨╗╤П ╨┐╨╡╤А╨╡╨║╨╗╤О╤З╨╡╨╜╨╕╤П ╤А╨░╨╜╨╛╨▓ ╨╜╨░
// ╨▓╨╜╨╡╨┤╤А╤С╨╜╨╜╤Л╨╡ ╤И╤А╨╕╤Д╤В╤Л (╤Д╨░╨╣╨╗╨╛╨▓╤Л╨╣ ╤Б╨╗╨╛╨╣). ╨а╨╡╨┤╨░╨║╤Ж╨╕╨╕ ╨▓╨╛╨╖╨▓╤А╨░╤Й╨░╤О╤В╤Б╤П ╨▓ ╨║╨╛╨╛╤А╨┤╨╕╨╜╨░╤В╨░╤Е
// ╨╕╤Б╤Е╨╛╨┤╨╜╨╛╨│╨╛ ╨┐╨╛╤В╨╛╨║╨░ ╨╕ ╨┐╤А╨╕╨╝╨╡╨╜╤П╤О╤В╤Б╤П ╨▓╤Л╨╖╤Л╨▓╨░╤О╤Й╨╡╨╣ ╤Б╤В╨╛╤А╨╛╨╜╨╛╨╣ ╤Б╨┐╤А╨░╨▓╨░ ╨╜╨░╨╗╨╡╨▓╨╛.
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

// tfTokens ╨╜╨░╤Е╨╛╨┤╨╕╤В ╨╕╨╜╨┤╨╡╨║╤Б╤Л ╤В╨╛╨║╨╡╨╜╨╛╨▓ ╨╕╨╝╨╡╨╜╨╕ ╨╕ ╤А╨░╨╖╨╝╨╡╤А╨░ ╨▓ ╨╛╨┐╨╡╤А╨░╤В╨╛╤А╨╡ Tf,
// ╨┤╨╡╨╣╤Б╤В╨▓╤Г╤О╤Й╨╡╨╝ ╨╜╨░ ╨╝╨╛╨╝╨╡╨╜╤В ╨┐╨╛╨║╨░╨╖╨░ ╤А╨░╨╜╨░ (╨┐╨╛╤Б╨╗╨╡╨┤╨╜╨╕╨╣ /F1 <size> Tf ╨┐╨╡╤А╨╡╨┤
// ╨╛╨┐╨╡╤А╨░╤В╨╛╤А╨╛╨╝ ╨┐╨╛╨║╨░╨╖╨░).
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

// editRange тАФ ╤В╨╛╤З╨╡╤З╨╜╨░╤П ╨┐╤А╨░╨▓╨║╨░ ╨▒╨░╨╣╤В╨╛╨▓╨╛╨│╨╛ ╨┤╨╕╨░╨┐╨░╨╖╨╛╨╜╨░ ╨╕╤Б╤Е╨╛╨┤╨╜╨╛╨│╨╛ ╨┐╨╛╤В╨╛╨║╨░.
// ╨Т╤Б╨╡ ╤А╨╡╨┤╨░╨║╤Ж╨╕╨╕ ╨┐╨╗╨░╨╜╨╕╤А╤Г╤О╤В╤Б╤П ╨▓ ╨║╨╛╨╛╤А╨┤╨╕╨╜╨░╤В╨░╤Е ╨Ш╨б╨е╨Ю╨Ф╨Э╨Ю╨У╨Ю ╨┐╨╛╤В╨╛╨║╨░ (╨┤╨╛ ╨╗╤О╨▒╤Л╤Е ╨┐╤А╨░╨▓╨╛╨║)
// ╨╕ ╨┐╤А╨╕╨╝╨╡╨╜╤П╤О╤В╤Б╤П ╨▓ Replace ╤Б╨┐╤А╨░╨▓╨░ ╨╜╨░╨╗╨╡╨▓╨╛ ╨┐╨╛ ╤Г╨▒╤Л╨▓╨░╨╜╨╕╤О start, ╨┐╨╛╤Н╤В╨╛╨╝╤Г ╨┐╨╛╨╖╨╕╤Ж╨╕╨╕
// ╨╜╨╡ ╨┐╨╛╨╡╤Е╨░╨╗╨╕ ╨┤╨░╨╢╨╡ ╨┐╤А╨╕ ╨╕╨╖╨╝╨╡╨╜╨╡╨╜╨╕╨╕ ╨┤╨╗╨╕╨╜╤Л ╤А╨░╨╜╨╜╨╕╤Е ╤Д╤А╨░╨│╨╝╨╡╨╜╤В╨╛╨▓.
type editRange struct {
	start, end int    // ╨▒╨░╨╣╤В╨╛╨▓╤Л╨╣ ╨┤╨╕╨░╨┐╨░╨╖╨╛╨╜ ╨▓ ╨╕╤Б╤Е╨╛╨┤╨╜╨╛╨╝ ╨┐╨╛╤В╨╛╨║╨╡
	to         string // ╤В╨╡╨║╤Б╤В ╨╖╨░╨╝╨╡╨╜╤Л
}

// RunCodec тАФ ╨┐╨╛╤Д╨░╨╣╨╗╨╛╨▓╨╛╨╡ ╨║╨╛╨┤╨╕╤А╨╛╨▓╨░╨╜╨╕╨╡ ╨╕ ╨╕╨╖╨╝╨╡╤А╨╡╨╜╨╕╨╡ ╤В╨╡╨║╤Б╤В╨░ ╤А╨░╨╜-╨▒╨╗╨╛╨║╨░ ╨┐╤А╨╕
// ╨┐╨╡╤А╨╡╨║╨╗╤О╤З╨╡╨╜╨╕╨╕ ╤А╨░╨╜╨░ ╨╜╨░ ╨▓╨╜╨╡╨┤╤А╤С╨╜╨╜╤Л╨╣ ╤И╤А╨╕╤Д╤В (╤Д╨░╨╣╨╗╨╛╨▓╤Л╨╣ ╤Б╨╗╨╛╨╣ ReplaceFile).
type RunCodec interface {
	// Encode ╨▓╨╛╨╖╨▓╤А╨░╤Й╨░╨╡╤В ╤Б╨╛╨┤╨╡╤А╨╢╨╕╨╝╨╛╨╡ ╤Б╤В╤А╨╛╨║╨╛╨▓╨╛╨│╨╛ ╨╗╨╕╤В╨╡╤А╨░╨╗╨░ (╨▒╨░╨╣╤В╤Л) ╨┤╨╗╤П utf8-╤В╨╡╨║╤Б╤В╨░.
	Encode(utf8 string) string
	// Hex тАФ true, ╨╡╤Б╨╗╨╕ Encode ╨▓╨╛╨╖╨▓╤А╨░╤Й╨░╨╡╤В ╨▒╨░╨╣╤В╤Л ╨┤╨▓╤Г╤Е╨▒╨░╨╣╤В╨╛╨▓╨╛╨╣ CID-╤Б╤В╤А╨╛╨║╨╕
	// Identity-H ╤И╤А╨╕╤Д╤В╨░; ╤В╨░╨║╨╛╨╣ ╨╗╨╕╤В╨╡╤А╨░╨╗ ╨┐╨╡╤З╨░╤В╨░╨╡╤В╤Б╤П ╨▓ ╤Д╨╛╤А╨╝╨╡ <...>.
	Hex() bool
	// Width тАФ ╤И╨╕╤А╨╕╨╜╨░ utf8 ╨▓ ╤В╤Л╤Б╤П╤З╨╜╤Л╤Е ╨┤╨╛╨╗╤П╤Е em ╨▓ ╨Ш╨б╨е╨Ю╨Ф╨Э╨Ю╨Ь ╤И╤А╨╕╤Д╤В╨╡ ╤А╨░╨╜╨░
	// (╨╝╨╡╤В╤А╨╕╨║╨╕ ╤В╨╛╨│╨╛ ╤И╤А╨╕╤Д╤В╨░, ╨║╨╛╤В╨╛╤А╤Л╨╝ ╨▒╤Л╨╗ ╨╜╨░╤А╨╕╤Б╨╛╨▓╨░╨╜ ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А).
	Width(utf8 string) float64
	// NewWidth тАФ ╤И╨╕╤А╨╕╨╜╨░ utf8 ╨▓ ╤В╤Л╤Б╤П╤З╨╜╤Л╤Е ╨┤╨╛╨╗╤П╤Е em ╨▓ ╨╜╨╛╨▓╨╛╨╝ (╨▓╨╜╨╡╨┤╤А╤П╨╡╨╝╨╛╨╝) ╤И╤А╨╕╤Д╤В╨╡.
	NewWidth(utf8 string) float64
	// FontName тАФ ╨╕╨╝╤П ╤А╨╡╤Б╤Г╤А╤Б╨░ ╨╜╨╛╨▓╨╛╨│╨╛ ╤И╤А╨╕╤Д╤В╨░ (/TF1) ╨┤╨╗╤П ╨┐╨╛╨┤╤Б╤В╨░╨╜╨╛╨▓╨║╨╕ ╨▓ Tf, ╨╗╨╕╨▒╨╛ "".
	FontName() string
}

// CodecProvider ╨▓╤Л╨▒╨╕╤А╨░╨╡╤В ╨║╨╛╨┤╨╕╤А╨╛╨▓╤Й╨╕╨║ ╨┤╨╗╤П ╤А╨░╨╜-╨▒╨╗╨╛╨║╨░. font тАФ ╨╕╨╝╤П ╤А╨╡╤Б╤Г╤А╤Б╨░
// ╨░╨║╤В╨╕╨▓╨╜╨╛╨│╨╛ ╤И╤А╨╕╤Д╤В╨░ ╤А╨░╨╜╨░ (/F1), text тАФ ╨╕╤В╨╛╨│╨╛╨▓╤Л╨╣ ╤В╨╡╨║╤Б╤В, ╨║╨╛╤В╨╛╤А╤Л╨╣ ╨▒╤Г╨┤╨╡╤В
// ╨╛╤В╤А╨╕╤Б╨╛╨▓╨░╨╜ ╤Н╤В╨╕╨╝ ╤А╨░╨╜╨╛╨╝. nil тАФ ╤А╨░╨╜ ╨╜╨╡ ╨┐╨╡╤А╨╡╨║╨╗╤О╤З╨░╤В╤М ╨╜╨░ ╨┤╤А╤Г╨│╨╛╨╣ ╤И╤А╨╕╤Д╤В
// (╤В╨╡╨║╤Б╤В ╨╛╤Б╤В╨░╤С╤В╤Б╤П ╨▓ ╨╕╤Б╤Е╨╛╨┤╨╜╨╛╨╝ ╤И╤А╨╕╤Д╤В╨╡, ╤И╨╕╤А╨╕╨╜╨░ тАФ ╤Н╨▓╤А╨╕╤Б╤В╨╕╤З╨╡╤Б╨║╨░╤П).
type CodecProvider func(font string, text string) RunCodec

// srcWidth ╨▓╨╛╨╖╨▓╤А╨░╤Й╨░╨╡╤В ╤И╨╕╤А╨╕╨╜╤Г ╤В╨╡╨║╤Б╤В╨░ ╨▓ ╨╕╤Б╤Е╨╛╨┤╨╜╨╛╨╝ ╤И╤А╨╕╤Д╤В╨╡ ╤А╨░╨╜╨░ (╨╕╨╗╨╕ ╤Н╨▓╤А╨╕╤Б╤В╨╕╨║╤Г).
func srcWidth(c RunCodec, s string) float64 {
	if c != nil {
		return c.Width(s)
	}
	return textWidth(s)
}

// dstWidth ╨▓╨╛╨╖╨▓╤А╨░╤Й╨░╨╡╤В ╤И╨╕╤А╨╕╨╜╤Г ╤В╨╡╨║╤Б╤В╨░ ╨▓ ╨╜╨╛╨▓╨╛╨╝ (╨▓╨╜╨╡╨┤╤А╤П╨╡╨╝╨╛╨╝) ╤И╤А╨╕╤Д╤В╨╡ (╨╕╨╗╨╕ ╤Н╨▓╤А╨╕╤Б╤В╨╕╨║╤Г).
func dstWidth(c RunCodec, s string) float64 {
	if c != nil {
		return c.NewWidth(s)
	}
	return textWidth(s)
}

// hexEncode ╨║╨╛╨┤╨╕╤А╤Г╨╡╤В ╨▒╨░╨╣╤В╤Л ╨▓ ╤И╨╡╤Б╤В╨╜╨░╨┤╤Ж╨░╤В╨╡╤А╨╕╤З╨╜╤Г╤О ╤Б╤В╤А╨╛╨║╤Г PDF-╨╗╨╕╤В╨╡╤А╨░╨╗╨░.
func hexEncode(b []byte) string {
	const hexd = "0123456789ABCDEF"
	var sb strings.Builder
	for _, c := range b {
		sb.WriteByte(hexd[c>>4])
		sb.WriteByte(hexd[c&0x0f])
	}
	return sb.String()
}

// renderOperand ╤Б╨╛╨▒╨╕╤А╨░╨╡╤В ╨╛╨┐╨╡╤А╨░╨╜╨┤ ╨╛╨┐╨╡╤А╨░╤В╨╛╤А╨░ ╨┐╨╛╨║╨░╨╖╨░ ╨┤╨╗╤П ╨╕╤В╨╛╨│╨╛╨▓╨╛╨│╨╛ ╤В╨╡╨║╤Б╤В╨░:
// string-╨╗╨╕╤В╨╡╤А╨░╨╗ (...) ╨╗╨╕╨▒╨╛ hex-╨╗╨╕╤В╨╡╤А╨░╨╗ <...> (╨┤╨╗╤П Identity-H CID-╤И╤А╨╕╤Д╤В╨╛╨▓),
// ╨┐╤А╨╕ ╨╜╨╡╨╛╨▒╤Е╨╛╨┤╨╕╨╝╨╛╤Б╤В╨╕ ╤Б ╤Е╨▓╨╛╤Б╤В╨╛╨▓╤Л╨╝ ╨║╨╡╤А╨╜╨╕╨╜╨│╨╛╨╝ ╨║╨╛╨╝╨┐╨╡╨╜╤Б╨░╤Ж╨╕╨╕ ╤И╨╕╤А╨╕╨╜╤Л.
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

// rebuildOperand ╨┐╨╡╤А╨╡╤Б╨╛╨▒╨╕╤А╨░╨╡╤В ╨╛╨┐╨╡╤А╨░╨╜╨┤ ╤А╨░╨╜-╨▒╨╗╨╛╨║╨░, ╨╖╨░╨╝╨╡╨╜╤П╤П ╨┐╨╛╨╗╤П ╨╖╨╜╨░╤З╨╡╨╜╨╕╤П╨╝╨╕.
// ╨Т╨╛╨╖╨▓╤А╨░╤Й╨░╨╡╤В ╨┐╤Г╤Б╤В╤Г╤О ╤Б╤В╤А╨╛╨║╤Г, ╨╡╤Б╨╗╨╕ ╨╖╨░╨╝╨╡╨╜╨░ ╨╜╨╡ ╨┐╤А╨╛╨╕╨╖╨╛╤И╨╗╨░ (╨║╨╗╤О╤З ╨╛╤В╤Б╤Г╤В╤Б╤В╨▓╤Г╨╡╤В ╨▓ values,
// ╨╕╨╗╨╕ тАФ ╨┐╤А╨╕ allowWider=false тАФ ╨╖╨╜╨░╤З╨╡╨╜╨╕╨╡ ╤И╨╕╤А╨╡ ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╨░).
// ╨Ю╨┐╨╡╤А╨░╨╜╨┤ ╤А╨╡╨╜╨┤╨╡╤А╨╕╤В╤Б╤П ╨║╨░╨║ TJ-╨╝╨░╤Б╤Б╨╕╨▓, ╨╡╤Б╨╗╨╕ ╨┐╨╛╤В╤А╨╡╨▒╨╛╨▓╨░╨╗╨░╤Б╤М ╨║╨╛╨╝╨┐╨╡╨╜╤Б╨░╤Ж╨╕╤П ╤И╨╕╤А╨╕╨╜╤Л;
// ╨╕╨╜╨░╤З╨╡ ╤Б╨╛╤Е╤А╨░╨╜╤П╨╡╤В╤Б╤П ╤Б╤В╤А╨╛╨║╨╛╨▓╨░╤П ╤Д╨╛╤А╨╝╨░ (... ) Tj.
//
// ╨Х╤Б╨╗╨╕ ╨┐╤А╨╛╨▓╨░╨╣╨┤╨╡╤А ╨║╨╛╨┤╨╡╨║╨╛╨▓ ╤А╨╡╤И╨╕╤В ╨┐╨╡╤А╨╡╨║╨╗╤О╤З╨╕╤В╤М ╤А╨░╨╜ ╨╜╨░ ╨▓╨╜╨╡╨┤╤А╤С╨╜╨╜╤Л╨╣ ╤И╤А╨╕╤Д╤В, ╨▓╨╛╨╖╨▓╤А╨░╤Й╨░╨╡╤В╤Б╤П
// ╨╜╨╡╨┐╤Г╤Б╤В╨╛╨╣ codec (╨╜╤Г╨╢╨╡╨╜ ╨▓╤Л╨╖╤Л╨▓╨░╤О╤Й╨╡╨╣ ╤Б╤В╨╛╤А╨╛╨╜╨╡ ╨┤╨╗╤П ╨┐╨╡╤А╨╡╨╕╨╝╨╡╨╜╨╛╨▓╨░╨╜╨╕╤П Tf). ╨а╨░╨╖╨╝╨╡╤А ╤И╤А╨╕╤Д╤В╨░
// ╨╕ ╨║╨╡╤А╨╜╨╕╨╜╨│ ╨║╨╛╨╜╤В╨╡╨║╤Б╤В╨░ (Tc/Tz/TJ) ╨╜╨╡ ╨╝╨╡╨╜╤П╤О╤В╤Б╤П: ╨╖╨╜╨░╤З╨╡╨╜╨╕╨╡ ╤А╨╡╨╜╨┤╨╡╤А╨╕╤В╤Б╤П ╤А╨░╨╖╨╝╨╡╤А╨╛╨╝
// ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╨░, ╨░╨▓╤В╨╛╤Г╤Б╨░╨┤╨║╨░ ╨┐╨╛╨┤ ╤И╨╕╤А╨╕╨╜╤Г ╨▒╨╛╨║╤Б╨░ ╨╜╨╡ ╨▓╤Л╨┐╨╛╨╗╨╜╤П╨╡╤В╤Б╤П.
//
// advPt тАФ ╨┐╨╛╨╗╨╜╨░╤П ╤И╨╕╤А╨╕╨╜╨░ ╨╕╤В╨╛╨│╨╛╨▓╨╛╨│╨╛ ╤В╨╡╨║╤Б╤В╨░ (finalText) ╨▓ ╤В╨╛╤З╨║╨░╤Е ╨┐╤А╨╕ ╤А╨░╨╖╨╝╨╡╤А╨╡ r.size;
// ╨╜╤Г╨╢╨╜╨░ ╨▓╤Л╨╖╤Л╨▓╨░╤О╤Й╨╡╨╣ ╤Б╤В╨╛╤А╨╛╨╜╨╡ ╨┤╨╗╤П ╤А╨░╤Б╤И╨╕╤А╨╡╨╜╨╕╤П ╨┐╨╛╨╗╤П (╨║╨╗╨╕╨┐-╨▒╨╛╨║╤Б╨░), ╤З╤В╨╛╨▒╤Л ╨┤╨╗╨╕╨╜╨╜╨╛╨╡
// ╨╖╨╜╨░╤З╨╡╨╜╨╕╨╡ ╨╜╨╡ ╨╛╨▒╤А╨╡╨╖╨░╨╗╨╛╤Б╤М ╨╕ ╨╜╨╡ ╨╜╨░╨╗╨╡╨╖╨░╨╗╨╛ ╨╜╨░ ╤Б╨╛╤Б╨╡╨┤╨╜╨╕╨╡ ╤Б╨╕╨╝╨▓╨╛╨╗╤Л.
func rebuildOperand(r streamRun, fields []field, values map[string]string, allowWider bool, provider CodecProvider) (operand string, codec RunCodec, advPt float64, err error) {
	segs := segOfRun(r, fields)
	if len(segs) == 0 {
		return "", nil, 0, nil
	}

	// ╨б╨╛╨▒╨╕╤А╨░╨╡╨╝ ╨╕╤В╨╛╨│╨╛╨▓╤Л╨╣ ╤В╨╡╨║╤Б╤В ╨╕ ╨┐╤А╨╛╨▓╨╡╤А╤П╨╡╨╝ ╨╜╨░╨╗╨╕╤З╨╕╨╡ ╨▓╤Б╨╡╤Е ╨╖╨╜╨░╤З╨╡╨╜╨╕╨╣.
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

	// ╨Я╨╡╤А╨╡╨║╨╗╤О╤З╨╡╨╜╨╕╨╡ ╤А╨░╨╜╨░ ╨╜╨░ ╨▓╨╜╨╡╨┤╤А╤С╨╜╨╜╤Л╨╣ ╤И╤А╨╕╤Д╤В, ╨╡╤Б╨╗╨╕ ╨╕╤Б╤Е╨╛╨┤╨╜╤Л╨╣ ╤И╤А╨╕╤Д╤В ╨╜╨╡ ╨┐╨╛╨║╤А╤Л╨▓╨░╨╡╤В
	// ╨╕╤В╨╛╨│╨╛╨▓╤Л╨╣ ╤В╨╡╨║╤Б╤В (╨╜╨░╨┐╤А╨╕╨╝╨╡╤А, ╨║╨╕╤А╨╕╨╗╨╗╨╕╤З╨╡╤Б╨║╨╕╨╡ ╨╖╨╜╨░╤З╨╡╨╜╨╕╤П ╨▓ ASCII-╤И╤А╨╕╤Д╤В╨╡ F1).
	if provider != nil {
		codec = provider(r.font, finalText)
	}

	// ╨Я╤А╨╛╨▓╨╡╤А╤П╨╡╨╝ ╤Б╤Г╨╝╨╝╨░╤А╨╜╤Г╤О ╨┤╨╡╨╗╤М╤В╤Г ╤И╨╕╤А╨╕╨╜╤Л ╨╕ ╨┤╨╛╨┐╤Г╤Б╤В╨╕╨╝╨╛╤Б╤В╤М ╨╖╨╜╨░╤З╨╡╨╜╨╕╤П.
	var delta float64
	for _, s := range segs {
		if s.kind == segText {
			continue
		}
		val := values[s.value]
		// ╨Ч╨╜╨░╤З╨╡╨╜╨╕╨╡ ╤И╨╕╤А╨╡ ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╨░ тАФ ╨╖╨░╨╝╨╡╨╜╤Г ╨╛╤В╨╝╨╡╨╜╤П╨╡╨╝ ╤Ж╨╡╨╗╨╕╨║╨╛╨╝,
		// ╨╡╤Б╨╗╨╕ ╤В╨╛╨╗╤М╨║╨╛ ╤П╨▓╨╜╨╛ ╨╜╨╡ ╤А╨░╨╖╤А╨╡╤И╨╡╨╜╨╛ (allowWider).
		if !allowWider && dstWidth(codec, val) > srcWidth(codec, s.text) {
			return "", nil, 0, nil
		}
		delta += srcWidth(codec, s.text) - dstWidth(codec, val)
	}

	return renderOperand(finalText, delta, codec), codec, dstWidth(codec, finalText) * r.size / 1000, nil
}

// segOfRun ╤А╨░╨╖╨▒╨╕╤А╨░╨╡╤В ╤В╨╡╨║╤Б╤В ╤А╨░╨╜-╨▒╨╗╨╛╨║╨░ ╨╜╨░ ╤Б╨╡╨│╨╝╨╡╨╜╤В╤Л ╨╝╨╡╨╢╨┤╤Г ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╨░╨╝╨╕.
type segKind uint8

const (
	segText segKind = iota
	segValue
)

type segment struct {
	kind  segKind
	text  string // ╨┤╨╗╤П segText тАФ ╨╜╨╡╨╕╨╖╨╝╨╡╨╜╨╜╤Л╨╣ ╤В╨╡╨║╤Б╤В; ╨┤╨╗╤П segValue тАФ ╨┐╨╛╨╗╨╜╤Л╨╣ ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А {{...}}
	begin int    // ╨▒╨░╨╣╤В╨╛╨▓╨░╤П ╨│╤А╨░╨╜╨╕╤Ж╨░ ╨▓ ╤А╨░╨╝╨║╨░╤Е ╨╛╨┐╨╡╤А╨░╨╜╨┤╨░ (╨╛╤В╨╜╨╛╤Б╨╕╤В╨╡╨╗╤М╨╜╨╛ r.start)
	end   int
	font  string
	size  float64
	value string // ╨┤╨╗╤П segValue тАФ ╨║╨╗╤О╤З ╨┤╨╗╤П ╨┐╨╛╨╕╤Б╨║╨░ ╨▓ values
}

// segOfRun ╤А╨╡╨╢╨╡╤В ╨╗╨╛╨│╨╕╤З╨╡╤Б╨║╨╕╨╣ ╤В╨╡╨║╤Б╤В ╤А╨░╨╜╨░ ╨╜╨░ ╤Б╨╡╨│╨╝╨╡╨╜╤В╤Л ╨┐╨╛ ╨│╤А╨░╨╜╨╕╤Ж╨░╨╝ ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╨╛╨▓,
// ╤Г╤З╨╕╤В╤Л╨▓╨░╤П ╤В╨╛╨╗╤М╨║╨╛ ╤В╨╡, ╤З╤В╨╛ ╤А╨╡╨░╨╗╤М╨╜╨╛ ╨▓╤Е╨╛╨┤╤П╤В ╨▓ ╤Б╨┐╨╕╤Б╨╛╨║ ╨┐╨╛╨╗╨╡╨╣ ╤А╨░╨╜╨░.
func segOfRun(r streamRun, fields []field) []segment {
	// ╨Ь╨╜╨╛╨╢╨╡╤Б╤В╨▓╨╛ ╨║╨╗╤О╤З╨╡╨╣ ╨┐╨╛╨╗╨╡╨╣ ╤А╨░╨╜╨░: ╨╖╨░╨╝╨╡╨╜╤П╨╡╨╝ ╤В╨╛╨╗╤М╨║╨╛ ╤В╨╛, ╤З╤В╨╛ ╨╜╨░╤И╤С╨╗ findFields
	// (╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╤Л ╨▓╨╜╤Г╤В╤А╨╕ range-╨▒╨╗╨╛╨║╨╛╨▓ ╤Б╤О╨┤╨░ ╨╜╨╡ ╨┐╨╛╨┐╨░╨┤╨░╤О╤В).
	known := make(map[string]bool, len(fields))
	for _, f := range fields {
		known[f.key] = true
	}
	var segs []segment
	pos := 0 // ╤В╨╡╨║╤Г╤Й╨░╤П ╨┐╨╛╨╖╨╕╤Ж╨╕╤П ╨▓ r.text
	for _, m := range placeholderRe.FindAllStringSubmatchIndex(r.text, -1) {
		// ╨Я╤А╨╛╨┐╤Г╤Б╨║╨░╨╡╨╝ ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╤Л, ╨╜╨╡ ╨┐╤А╨╕╨╜╨░╨┤╨╗╨╡╨╢╨░╤Й╨╕╨╡ ╤Б╨┐╨╕╤Б╨║╤Г ╨┐╨╛╨╗╨╡╨╣ (╨╜╨░╨┐╤А╨╕╨╝╨╡╤А,
		// ╨╕╨╖ range-╨▒╨╗╨╛╨║╨╛╨▓, ╨║╨╛╤В╨╛╤А╤Л╨╡ ╨╕╨│╨╜╨╛╤А╨╕╤А╤Г╤О╤В╤Б╤П).
		if m[0] < pos {
			continue
		}
		key := strings.TrimSpace(r.text[m[2]:m[3]])
		if !known[key] {
			// ╨Э╨╡ ╨▓ ╤Б╨┐╨╕╤Б╨║╨╡ тАФ ╨▓╨║╨╗╤О╤З╨░╨╡╨╝ ╨║╨░╨║ ╨╛╨▒╤Л╤З╨╜╤Л╨╣ ╤В╨╡╨║╤Б╤В, ╨╜╨╡ ╨┐╨╡╤А╨╡╤Б╨╛╨▒╨╕╤А╨░╤П.
			segs = append(segs, segment{
				kind: segText,
				text: r.text[pos:m[1]],
			})
			pos = m[1]
			continue
		}
		// ╨е╨▓╨╛╤Б╤В ╤В╨╡╨║╤Б╤В╨░ ╨┐╨╡╤А╨╡╨┤ ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╨╛╨╝.
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
		// ╨б╨░╨╝ ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А: ╤И╨╕╤А╨╕╨╜╨░ ╤Г╤З╨╕╤В╤Л╨▓╨░╨╡╤В╤Б╤П ╤Ж╨╡╨╗╨╕╨║╨╛╨╝ ╨▓╨╝╨╡╤Б╤В╨╡ ╤Б╨╛ ╤Б╨║╨╛╨▒╨║╨░╨╝╨╕,
		// ╨▓╨╡╨┤╤М ╨╕╨╝╨╡╨╜╨╜╨╛ ╨╛╨╜ ╨▒╤Л╨╗ ╨╛╤В╤А╨╕╤Б╨╛╨▓╨░╨╜ ╨▓ PDF ╨╕ ╨╖╨░╨╜╨╕╨╝╨░╨╗ ╨╝╨╡╤Б╤В╨╛.
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
	// ╨е╨▓╨╛╤Б╤В ╤В╨╡╨║╤Б╤В╨░ ╨┐╨╛╤Б╨╗╨╡ ╨┐╨╛╤Б╨╗╨╡╨┤╨╜╨╡╨│╨╛ ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╨░.
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

// quoteString ╤Н╨║╤А╨░╨╜╨╕╤А╤Г╨╡╤В ╤Б╤В╤А╨╛╨║╤Г ╨▓ PDF-╨╗╨╕╤В╨╡╤А╨░╨╗ (╤Б╨║╨╛╨▒╨║╨╕ ╨╕ ╨╛╨▒╤А╨░╤В╨╜╨░╤П ╨║╨╛╤Б╨░╤П).
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

// applyEdits ╨┐╤А╨╕╨╝╨╡╨╜╤П╨╡╤В ╨┐╤А╨░╨▓╨║╨╕ ╨║ ╨┐╨╛╤В╨╛╨║╤Г ╤Б╨┐╤А╨░╨▓╨░ ╨╜╨░╨╗╨╡╨▓╨╛ (╨┐╨╛ ╤Г╨▒╤Л╨▓╨░╨╜╨╕╤О start):
// ╤А╨░╤Б╤И╨╕╤А╨╡╨╜╨╕╨╡/╤Б╨╢╨░╤В╨╕╨╡ ╤А╨░╨╜╨╜╨╕╤Е ╤Д╤А╨░╨│╨╝╨╡╨╜╤В╨╛╨▓ ╨╜╨╡ ╤Б╨┤╨▓╨╕╨│╨░╨╡╤В ╨┐╨╛╨╖╨╕╤Ж╨╕╨╕ ╨┐╨╛╨╖╨┤╨╜╨╕╤Е.
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

// ReplaceParams тАФ ╨┐╨░╤А╨░╨╝╨╡╤В╤А╤Л ╨╖╨░╨╝╨╡╨╜╤Л ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╨╛╨▓ ╨▓ ╨║╨╛╨╜╤В╨╡╨╜╤В╨╜╨╛╨╝ ╨┐╨╛╤В╨╛╨║╨╡ PDF.
// ╨Р╨╜╨░╨╗╨╛╨│ WconvPipeline ╨╕╨╖ pkg/convert, ╨╜╨╛ ╨┤╨╗╤П pdfpl.
type ReplaceParams struct {
	Stream string            // ╤Б╨╛╨┤╨╡╤А╨╢╨╕╨╝╨╛╨╡ ╨║╨╛╨╜╤В╨╡╨╜╤В╨╜╨╛╨│╨╛ ╨┐╨╛╤В╨╛╨║╨░
	Values map[string]string // ╨║╨╗╤О╤З тЖТ ╨╖╨╜╨░╤З╨╡╨╜╨╕╨╡ (╨│╨╛╨╗╤Л╨╡ ╨║╨╗╤О╤З╨╕, ╨▒╨╡╨╖ ╤В╨╛╤З╨║╨╕)

	// AllowWider ╤А╨░╨╖╤А╨╡╤И╨░╨╡╤В ╨╖╨░╨╝╨╡╨╜╤Г ╨╖╨╜╨░╤З╨╡╨╜╨╕╤П╨╝╨╕, ╤И╨╕╤А╨╡ ╨╕╤Б╤Е╨╛╨┤╨╜╨╛╨│╨╛ ╤В╨╡╨║╤Б╤В╨░
	// ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╨░ (╤В╨╡╨║╤Б╤В ╨▓╤Л╤Е╨╛╨┤╨╕╤В ╨╖╨░ ╨┐╤А╨╡╨┤╨╡╨╗╤Л ╨╕╤Б╤Е╨╛╨┤╨╜╨╛╨╣ ╨╗╨╕╨╜╨╕╨╕ ╨▓╨┐╤А╨░╨▓╨╛).
	// ╨Я╨╛ ╤Г╨╝╨╛╨╗╤З╨░╨╜╨╕╤О (false) ╤В╨░╨║╨╕╨╡ ╨╖╨╜╨░╤З╨╡╨╜╨╕╤П ╨┐╤А╨╛╨┐╤Г╤Б╨║╨░╤О╤В╤Б╤П. ╨Ф╨╗╤П ╤Д╨░╨╣╨╗╨╛╨▓╨╛╨│╨╛
	// ╤Б╨╗╨╛╤П (ReplaceFile) ╨▓╤Б╨╡╨│╨┤╨░ ╨▓╨║╨╗╤О╤З╤С╨╜.
	AllowWider bool

	// Codec тАФ ╨╜╨╡╨╛╨▒╤П╨╖╨░╤В╨╡╨╗╤М╨╜╤Л╨╣ ╨┐╤А╨╛╨▓╨░╨╣╨┤╨╡╤А ╨║╨╛╨┤╨╡╨║╨╛╨▓ ╨┤╨╗╤П ╨┐╨╡╤А╨╡╨║╨╗╤О╤З╨╡╨╜╨╕╤П ╤А╨░╨╜╨╛╨▓ ╨╜╨░
	// ╨▓╨╜╨╡╨┤╤А╤С╨╜╨╜╤Л╨╡ ╤И╤А╨╕╤Д╤В╤Л (╤Д╨░╨╣╨╗╨╛╨▓╤Л╨╣ ╤Б╨╗╨╛╨╣ ReplaceFile). nil тАФ ╤А╨░╨╜╤Л ╨╛╤Б╤В╨░╤О╤В╤Б╤П ╨▓
	// ╨╕╤Б╤Е╨╛╨┤╨╜╤Л╤Е ╤И╤А╨╕╤Д╤В╨░╤Е, ╤И╨╕╤А╨╕╨╜╨░ ╤В╨╡╨║╤Б╤В╨░ ╤Б╤З╨╕╤В╨░╨╡╤В╤Б╤П ╤Н╨▓╤А╨╕╤Б╤В╨╕╤З╨╡╤Б╨║╨╕.
	Codec CodecProvider

	// Decode тАФ ╨╜╨╡╨╛╨▒╤П╨╖╨░╤В╨╡╨╗╤М╨╜╨░╤П ╤Д╤Г╨╜╨║╤Ж╨╕╤П ╤В╤А╨░╨╜╤Б╨╗╤П╤Ж╨╕╨╕ ╨╗╨╛╨│╨╕╤З╨╡╤Б╨║╨╛╨│╨╛ ╤В╨╡╨║╤Б╤В╨░ ╤А╨░╨╜╨░ ╨┐╨╡╤А╨╡╨┤
	// ╨┐╨╛╨╕╤Б╨║╨╛╨╝ ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╨╛╨▓. ╨Я╤А╨╕╨╝╨╡╨╜╤П╨╡╤В╤Б╤П ╨║ ╤А╨░╨╜╤Г (font тАФ ╨░╨║╤В╨╕╨▓╨╜╤Л╨╣ ╤И╤А╨╕╤Д╤В /F1),
	// ╤З╨╡╨╣ ╤В╨╡╨║╤Б╤В ╨╖╨░╨║╨╛╨┤╨╕╤А╨╛╨▓╨░╨╜ ╨▒╨░╨╣╤В╨░╨╝╨╕-╤Б╨╕╨╝╨▓╨╛╨╗╨░╨╝╨╕ (╨║╨╛╨╝╨┐╨╛╨╖╨╕╤В╨╜╤Л╨╡ Identity-H ╤И╤А╨╕╤Д╤В╤Л ╤Б
	// ToUnicode-╨║╨░╤А╤В╨╛╨╣, ╤В╨╕╨┐╨╕╤З╨╜╨╛ ╨┤╨╗╤П ╤Б╨░╨▒╤Б╨╡╤В╨╛╨▓ MS Word): hex-CID ╨▒╨░╨╣╤В╤Л ╨┐╨╡╤А╨╡╨▓╨╛╨┤╤П╤В╤Б╤П
	// ╨▓ Unicode, ╤З╤В╨╛╨▒╤Л findFields ╤Г╨▓╨╕╨┤╨╡╨╗ {{.Key}}. ╨Ф╨╗╤П ╤И╤А╨╕╤Д╤В╨╛╨▓, ╨╜╨╡ ╤В╤А╨╡╨▒╤Г╤О╤Й╨╕╤Е
	// ╨┤╨╡╨║╨╛╨┤╨╕╤А╨╛╨▓╨░╨╜╨╕╤П, ╨▓╨╛╨╖╨▓╤А╨░╤Й╨░╨╡╤В ╨╕╤Б╤Е╨╛╨┤╨╜╤Л╨╣ ╤В╨╡╨║╤Б╤В. nil тАФ ╨╜╨╡ ╨┤╨╡╨║╨╛╨┤╨╕╤А╨╛╨▓╨░╤В╤М.
	Decode func(font, text string) string
}

// ReplaceResult тАФ ╤А╨╡╨╖╤Г╨╗╤М╤В╨░╤В ╨╖╨░╨╝╨╡╨╜╤Л ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╨╛╨▓ ╨▓ ╨║╨╛╨╜╤В╨╡╨╜╤В╨╜╨╛╨╝ ╨┐╨╛╤В╨╛╨║╨╡.
type ReplaceResult struct {
	Stream   string // ╨╝╨╛╨┤╨╕╤Д╨╕╤Ж╨╕╤А╨╛╨▓╨░╨╜╨╜╤Л╨╣ ╨║╨╛╨╜╤В╨╡╨╜╤В╨╜╤Л╨╣ ╨┐╨╛╤В╨╛╨║
	Replaced int    // ╨║╨╛╨╗╨╕╤З╨╡╤Б╤В╨▓╨╛ ╨╖╨░╨╝╨╡╨╜╤С╨╜╨╜╤Л╤Е ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╨╛╨▓
	Skipped  int    // ╨║╨╛╨╗╨╕╤З╨╡╤Б╤В╨▓╨╛ ╨┐╤А╨╛╨┐╤Г╤Й╨╡╨╜╨╜╤Л╤Е (╨║╨╗╤О╤З ╨╜╨╡ ╨╜╨░╨╣╨┤╨╡╨╜ ╨╕╨╗╨╕ ╨╖╨╜╨░╤З╨╡╨╜╨╕╨╡ ╤И╨╕╤А╨╡)
}

// Replace ╨╖╨░╨╝╨╡╨╜╤П╨╡╤В ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╤Л {{.Key}}/{{Key}} ╨▓ ╨║╨╛╨╜╤В╨╡╨╜╤В╨╜╨╛╨╝ ╨┐╨╛╤В╨╛╨║╨╡ PDF
// ╨╖╨╜╨░╤З╨╡╨╜╨╕╤П╨╝╨╕ ╨╕╨╖ Values. ╨Ч╨░╨╝╨╡╨╜╨░ ╤Б╨╛╤Е╤А╨░╨╜╤П╨╡╤В ╤И╨╕╤А╨╕╨╜╤Г ╤В╨╡╨║╤Б╤В╨░: ╤А╨░╨╖╨╜╨╕╤Ж╨░ ╨▓ ╤И╨╕╤А╨╕╨╜╨╡
// ╨║╨╛╨╝╨┐╨╡╨╜╤Б╨╕╤А╤Г╨╡╤В╤Б╤П ╤З╨╕╤Б╨╗╨╛╨╝ ╨║╨╡╤А╨╜╨╕╨╜╨│╨░ ╨▓ TJ-╨╝╨░╤Б╤Б╨╕╨▓╨╡.
//
// ╨Я╨╛╨┤╨┤╨╡╤А╨╢╨╕╨▓╨░╨╡╤В ╨║╨░╨║ ╨╛╨┤╨╜╨╛╤А╨░╨╜╨╛╨▓╤Л╨╡, ╤В╨░╨║ ╨╕ ╨╝╨╡╨╢╤А╨░╨╜╨╛╨▓╤Л╨╡ (cross-run) ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╤Л,
// ╤А╨░╨╖╨╜╨╡╤Б╤С╨╜╨╜╤Л╨╡ ╨┐╨╛ ╨╜╨╡╤Б╨║╨╛╨╗╤М╨║╨╕╨╝ BT/ET-╨▒╨╗╨╛╨║╨░╨╝ (╤В╨╕╨┐╨╕╤З╨╜╨╛ ╨┤╨╗╤П PDF ╨╛╤В MS Word).
// ╨Я╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А, ╨║╨╗╤О╤З ╨║╨╛╤В╨╛╤А╨╛╨│╨╛ ╨╛╤В╤Б╤Г╤В╤Б╤В╨▓╤Г╨╡╤В ╨▓ Values ╨╕╨╗╨╕ ╨╖╨╜╨░╤З╨╡╨╜╨╕╨╡ ╨║╨╛╤В╨╛╤А╨╛╨│╨╛ ╤И╨╕╤А╨╡
// ╨╕╤Б╤Е╨╛╨┤╨╜╨╛╨│╨╛ ╤В╨╡╨║╤Б╤В╨░, ╨╛╤Б╤В╨░╤С╤В╤Б╤П ╨╜╨╡ ╤В╤А╨╛╨╜╤Г╤В╤Л╨╝ ╨╕ ╤Г╤З╨╕╤В╤Л╨▓╨░╨╡╤В╤Б╤П ╨▓ Skipped.
//
// ╨д╨╛╤А╨╝╨░╤В ╤А╨╡╨╖╤Г╨╗╤М╤В╨░╤В╨░ ╤Б╨╛╨▓╨╝╨╡╤Б╤В╨╕╨╝ ╤Б ╨║╨╛╨╜╨▓╨╡╨╜╤Ж╨╕╤П╨╝╨╕ wconv: ╤З╨╕╤Б╤В╨░╤П ╤Д╤Г╨╜╨║╤Ж╨╕╤П ╨▒╨╡╨╖
// ╨┐╨╛╨▒╨╛╤З╨╜╤Л╤Е ╤Н╤Д╤Д╨╡╨║╤В╨╛╨▓, ╨┐╤А╨╕╨╜╨╕╨╝╨░╨╡╤В ╨┐╨░╤А╨░╨╝╨╡╤В╤А╤Л ╤Б╤В╤А╤Г╨║╤В╤Г╤А╨╜╨╛ ╨╕ ╨▓╨╛╨╖╨▓╤А╨░╤Й╨░╨╡╤В ╤А╨╡╨╖╤Г╨╗╤М╤В╨░╤В.
func Replace(params *ReplaceParams) (*ReplaceResult, error) {
	if params == nil || params.Stream == "" {
		return &ReplaceResult{Stream: params.Stream, Replaced: 0, Skipped: 0}, nil
	}

	runs, err := textRuns(params.Stream)
	if err != nil {
		return nil, fmt.Errorf("╤А╨░╨╖╨▒╨╛╤А ╨║╨╛╨╜╤В╨╡╨╜╤В╨╜╨╛╨│╨╛ ╨┐╨╛╤В╨╛╨║╨░: %w", err)
	}

	// ╨в╤А╨░╨╜╤Б╨╗╤П╤Ж╨╕╤П ╨╗╨╛╨│╨╕╤З╨╡╤Б╨║╨╛╨│╨╛ ╤В╨╡╨║╤Б╤В╨░ ╤А╨░╨╜╨╛╨▓ (hex-CID тЖТ Unicode ╨┤╨╗╤П Identity-H
	// ╤Б╨░╨▒╤Б╨╡╤В╨╛╨▓) ╨┤╨╛ ╨┐╨╛╨╕╤Б╨║╨░ ╨┐╨╛╨╗╨╡╨╣, ╤З╤В╨╛╨▒╤Л {{.Key}} ╨▒╤Л╨╗╨╕ ╨▓╨╕╨┤╨╜╤Л findFields.
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

// ListPlaceholders ╨▓╨╛╨╖╨▓╤А╨░╤Й╨░╨╡╤В ╨╕╨╝╨╡╨╜╨░ ╨▓╤Б╨╡╤Е ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╨╛╨▓ {{.Key}}/{{Key}},
// ╨╜╨░╨╣╨┤╨╡╨╜╨╜╤Л╤Е ╨▓ ╨║╨╛╨╜╤В╨╡╨╜╤В╨╜╨╛╨╝ ╨┐╨╛╤В╨╛╨║╨╡: ╨╕ ╨╛╨┤╨╜╨╛╤А╨░╨╜╨╛╨▓╤Л╤Е, ╨╕ ╨╝╨╡╨╢╤А╨░╨╜╨╛╨▓╤Л╤Е (cross-run).
// ╨Я╨╛╨╗╨╡╨╖╨╜╨╛ ╨┤╨╗╤П ╨┤╨╕╨░╨│╨╜╨╛╤Б╤В╨╕╨║╨╕/╤Б╤Г╤Е╨╛╨│╨╛ ╨┐╤А╨╛╨│╨╛╨╜╨░: ╨║╨░╨║╨╕╨╡ ╨║╨╗╤О╤З╨╕ ╤И╨░╨▒╨╗╨╛╨╜ ╨╢╨┤╤С╤В ╨╕╨╖ ╨┤╨░╨╜╨╜╤Л╤Е.
func ListPlaceholders(stream string) ([]string, error) {
	runs, err := textRuns(stream)
	if err != nil {
		return nil, fmt.Errorf("╤А╨░╨╖╨▒╨╛╤А ╨║╨╛╨╜╤В╨╡╨╜╤В╨╜╨╛╨│╨╛ ╨┐╨╛╤В╨╛╨║╨░: %w", err)
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
