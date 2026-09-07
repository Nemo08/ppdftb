// Package pdfpl предоставляет шаблонизатор PDF (аналог go-template-docx,
// но для статических PDF-шаблонов): подставляет значения из данных
// в плейсхолдеры {{.Key}} контентных потоков. Range-блоки
// {{range ...}}...{{end}} игнорируются целиком.
package pdfpl

import (
	"fmt"
	"strconv"
	"strings"
)

// tokenKind — лексическая категория токена контентного потока.
type tokenKind uint8

const (
	tokNum  tokenKind = iota // число, напр. -0.012 или 18
	tokStr                   // литеральная строка (abc)
	tokHex                   // шестнадцатеричная строка <...>
	tokName                  // имя /F1
	tokOpen                  // '[' начало массива
	tokClose                 // ']' конец массива
	tokOp                    // оператор (Tj, TJ, Tf, BT ...)
)

// token — элемент контентного потока с привязкой к байтовому диапазону.
type token struct {
	kind  tokenKind
	text  string // содержимое (строка/оператор/имя)
	start int    // абсолютное смещение начала в потоке
	end   int    // абсолютное смещение конца (исключительно)
}

// streamRun — фрагмент показанного текста в логической последовательности
// с байтовым диапазоном в пределах всего потока и активным шрифтом на момент показа.
type streamRun struct {
	text   string  // отображаемый текст (логическая последовательность)
	start  int     // начало байтового диапазона
	end    int     // конец байтового диапазона (исключительно)
	font   string  // имя активного шрифта (/F1)
	size   float64 // размер шрифта из Tf
	opTok  int     // индекс токена оператора показа (Tj/TJ) в списке токенов
	opType string  // тип оператора: "Tj" или "TJ"
}

// lexer — состояние разбора контентного потока.
type lexer struct {
	data   []byte
	i      int
	tokens []token
	err    error
}

// tokenize разбирает контентный поток на токены.
func tokenize(stream string) ([]token, error) {
	lx := &lexer{data: []byte(stream)}
	for lx.i < len(lx.data) && lx.err == nil {
		c := lx.data[lx.i]
		switch {
		case isWs(c):
			lx.i++
		case c == '(':
			lx.lexString()
		case c == '<':
			lx.lexAngle()
		case c == ')':
			lx.err = fmt.Errorf("неожиданная скобка ')' на позиции %d", lx.i)
		case c == '[':
			lx.push(tokOpen, "[", lx.i, lx.i+1)
			lx.i++
		case c == ']':
			lx.push(tokClose, "]", lx.i, lx.i+1)
			lx.i++
		case c == '/':
			lx.lexName()
		case c == '%':
			lx.skipComment()
		default:
			lx.lexWord()
		}
	}
	if lx.err != nil {
		return nil, lx.err
	}
	return lx.tokens, nil
}

// push добавляет токен в список.
func (lx *lexer) push(kind tokenKind, text string, start, end int) {
	lx.tokens = append(lx.tokens, token{kind: kind, text: text, start: start, end: end})
}

func isWs(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

func isDelim(c byte) bool {
	return c == '(' || c == ')' || c == '<' || c == '>' || c == '[' || c == ']' ||
		c == '{' || c == '}' || c == '/' || c == '%'
}

// lexString парсит литеральную строку вида (abc) с учётом экранирования.
func (lx *lexer) lexString() {
	start := lx.i
	lx.i++ // пропускаем (
	var sb strings.Builder
	for lx.i < len(lx.data) {
		c := lx.data[lx.i]
		if c == '\\' {
			lx.bindEscape(&sb)
			continue
		}
		if c == ')' {
			lx.i++
			lx.tokens = append(lx.tokens, token{kind: tokStr, text: sb.String(), start: start, end: lx.i})
			return
		}
		sb.WriteByte(c)
		lx.i++
	}
	lx.err = fmt.Errorf("незакрытая литеральная строка на позиции %d", start)
}

// bindEscape разворачивает escape-последовательность в литеральной строке.
func (lx *lexer) bindEscape(sb *strings.Builder) {
	lx.i++ // символ backslash
	if lx.i >= len(lx.data) {
		return
	}
	c := lx.data[lx.i]
	switch c {
	case 'n', 'r', 't', 'b', 'f':
		sb.WriteByte(escapeByte(c))
		lx.i++
	case '(', ')', '\\':
		sb.WriteByte(c)
		lx.i++
	default:
		if isOctDigit(c) {
			var code byte
			for lx.i < len(lx.data) && isOctDigit(lx.data[lx.i]) {
				code = code*8 + (lx.data[lx.i] - '0')
				lx.i++
			}
			sb.WriteByte(code)
			return
		}
		sb.WriteByte(c)
		lx.i++
	}
}

func escapeByte(c byte) byte {
	return map[byte]byte{'n': '\n', 'r': '\r', 't': '\t', 'b': '\b', 'f': '\f'}[c]
}

func isOctDigit(c byte) bool { return c >= '0' && c <= '7' }

// lexAngle обрабатывает конструкцию <...> (<hex> либо <<...>>).
func (lx *lexer) lexAngle() {
	if lx.i+1 < len(lx.data) && lx.data[lx.i+1] == '<' {
		lx.err = fmt.Errorf("словарь в контентном потоке не поддерживается на позиции %d", lx.i)
		return
	}
	start := lx.i
	lx.i++ // пропускаем <
	var hex strings.Builder
	for lx.i < len(lx.data) {
		c := lx.data[lx.i]
		if c == '>' {
			lx.i++
			body, err := decodeHex(hex.String())
			if err != nil {
				lx.err = fmt.Errorf("шестнадцатеричная строка: %w", err)
				return
			}
			lx.tokens = append(lx.tokens, token{kind: tokHex, text: body, start: start, end: lx.i})
			return
		}
		if !isWs(c) {
			hex.WriteByte(c)
		}
		lx.i++
	}
	lx.err = fmt.Errorf("незакрытая шестнадцатеричная строка на позиции %d", start)
}

// lexName парсит имя /F1 (включая косую черту).
func (lx *lexer) lexName() {
	start := lx.i
	lx.i++ // косая черта
	for lx.i < len(lx.data) && !isDelim(lx.data[lx.i]) {
		lx.i++
	}
	lx.tokens = append(lx.tokens, token{kind: tokName, text: string(lx.data[start:lx.i]), start: start, end: lx.i})
}

// lexWord обрабатывает число либо оператор (слово до разделителя).
func (lx *lexer) lexWord() {
	start := lx.i
	for lx.i < len(lx.data) && !isDelim(lx.data[lx.i]) {
		lx.i++
	}
	word := string(lx.data[start:lx.i])
	kind := tokOp
	if isNumber(word) {
		kind = tokNum
	}
	lx.tokens = append(lx.tokens, token{kind: kind, text: word, start: start, end: lx.i})
}

// skipComment пропускает комментарий % до конца строки.
func (lx *lexer) skipComment() {
	for lx.i < len(lx.data) && lx.data[lx.i] != '\n' && lx.data[lx.i] != '\r' {
		lx.i++
	}
}

func decodeHex(s string) (string, error) {
	if len(s)%2 == 1 {
		s += "0"
	}
	out := make([]byte, 0, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		hi, e1 := hexNibble(s[i])
		lo, e2 := hexNibble(s[i+1])
		if e1 != nil || e2 != nil {
			return "", fmt.Errorf("недопустимый hex-символ")
		}
		out = append(out, hi<<4|lo)
	}
	return string(out), nil
}

func hexNibble(c byte) (byte, error) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', nil
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, nil
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, nil
	}
	return 0, fmt.Errorf("недопустимый hex-символ %q", c)
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '+' || s[0] == '-' {
		i++
	}
	dot := false
	for ; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c == '.' && !dot:
			dot = true
		default:
			return false
		}
	}
	return true
}

// parseFontSize выделяет из последовательности Tf параметры (имя, размер).
// Операнды Tf идут так: имя размер Tf — это два токена перед оператором.
func parseFontSize(toks []token, op int) (string, float64, bool) {
	if op < 2 || toks[op-1].kind != tokNum || toks[op-2].kind != tokName {
		return "", 0, false
	}
	size, err := strconv.ParseFloat(toks[op-1].text, 64)
	if err != nil {
		return "", 0, false
	}
	return toks[op-2].text, size, true
}

// operandBounds возвращает байтовые границы операнда оператора показа:
// для TJ от '[' до конца оператора, для Tj от строки до конца оператора.
func operandBounds(toks []token, op int) (int, int, bool) {
	if op < 1 {
		return 0, 0, false
	}
	if toks[op].text == "Tj" {
		t := toks[op-1]
		if t.kind != tokHex && t.kind != tokStr {
			return 0, 0, false
		}
		return t.start, toks[op].end, true
	}
	// TJ: ищем '[' в обратную сторону.
	for p := op - 1; p >= 0; p-- {
		if toks[p].kind == tokOpen {
			return toks[p].start, toks[op].end, true
		}
	}
	return 0, 0, false
}

// extractShowText собирает текст показанного оператора (Tj или TJ).
func extractShowText(toks []token, op int) string {
	if toks[op].text == "Tj" {
		if toks[op-1].kind == tokStr || toks[op-1].kind == tokHex {
			return toks[op-1].text
		}
		return ""
	}
	var sb strings.Builder
	for p := op - 1; p >= 0; p-- {
		if toks[p].kind == tokOpen {
			break
		}
		if toks[p].kind == tokStr || toks[p].kind == tokHex {
			sb.WriteString(toks[p].text)
		}
	}
	return sb.String()
}

// textRuns извлекает показанные фрагменты текста вместе со шрифтом.
func textRuns(stream string) ([]streamRun, error) {
	toks, err := tokenize(stream)
	if err != nil {
		return nil, err
	}
	var runs []streamRun
	inText := false
	font := ""
	size := 0.0
	for op := 0; op < len(toks); op++ {
		t := toks[op]
		if t.kind != tokOp {
			continue
		}
		switch t.text {
		case "BT":
			inText = true
		case "ET":
			inText = false
		case "Tf":
			if f, s, ok := parseFontSize(toks, op); ok {
				font, size = f, s
			}
		case "Tj", "TJ":
			if !inText {
				continue
			}
			start, end, ok := operandBounds(toks, op)
			if !ok {
				continue
			}
			runs = append(runs, streamRun{
				text:  extractShowText(toks, op),
				start: start,
				end:   end,
				font:  font,
				size:  size,
			})
		}
	}
	return runs, nil
}