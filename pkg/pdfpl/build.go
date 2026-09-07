package pdfpl

import (
	"fmt"
	"strings"
)

// buildStream заменяет плейсхолдеры в контентном потоке значениями из values.
// Плейсхолдер, значение которого шире исходного текста, остаётся не тронутым.
// Замена сохраняет ширину текста: разница в ширине компенсируется числом
// кернинга в TJ-массиве, чтобы следующий текст не поехал.
func buildStream(stream string, runs []streamRun, fields []field, values map[string]string) (string, error) {
	if len(fields) == 0 {
		return stream, nil
	}
	// Группируем поля по ранам — каждый ран пересобирается целиком.
	byRun := map[int][]field{}
	for _, f := range fields {
		byRun[f.run] = append(byRun[f.run], f)
	}
	// Применяем правки с конца потока, чтобы смещения не поехали.
	var newEnds []int // начала правок в порядке от конца
	modified := stream
	for _, r := range runs {
		fl := byRun[r.run]
		if len(fl) == 0 {
			continue
		}
		newOperand, err := rebuildOperand(stream, r, fl, values)
		if err != nil {
			return "", err
		}
		if newOperand == "" {
			continue
		}
		// actual bytes of the operand in the original stream
		_ = newEnds
		modified = padReplace(modified, r.start, r.end, newOperand)
	}
	return modified, nil
}

// padReplace заменяет срез [lo, hi) строки s на replacement.
func padReplace(s string, lo, hi int, replacement string) string {
	return s[:lo] + replacement + s[hi:]
}

// rebuildOperand пересобирает операнд ран-блока, заменяя поля значениями.
// Возвращает пустую строку, если замены не произошло.
// Операнд рендерится как TJ-массив, если в нём была строка и потребовалась
// компенсация ширины; иначе сохраняется строковая форма (… ) Tj.
func rebuildOperand(stream string, r streamRun, fields []field, values map[string]string) (string, error) {
	// Разбиваем текст рана на сегменты по границам плейсхолдеров.
	seg := segOfRun(r, fields)
	_ = seg
	return "", fmt.Errorf("not implemented")
}

// segOfRun разбирает текст ран-блока на сегменты между плейсхолдерами.
type segKind uint8

const (
	segText segKind = iota
	segValue
)

type segment struct {
	kind     segKind
	text     string
	begin    int // байтовая граница в рамках операнда (относительно r.start)
	end      int
	font     string
	size     float64
	value    string // для segValue — значение из данных
	placeLen int    // len placeText
}

func segEndSegment(r streamRun, _ streamRun) []segment {
	return nil
}

// …