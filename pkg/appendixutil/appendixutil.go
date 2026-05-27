// Package appendixutil содержит чистые функции для работы с приложениями ГОСТ Р 2.105-2019.
// Вынесен отдельно чтобы тестироваться без зависимости от PDF-библиотек.
package appendixutil

import "strings"

// Letters — буквы для нумерации приложений по ГОСТ Р 2.105-2019.
// Исключены: Ё, З, Й, О, Ч, Ъ, Ы, Ь.
var Letters = []string{
	"А", "Б", "В", "Г", "Д", "Е", "Ж",
	"И", "К", "Л", "М", "Н", "П", "Р",
	"С", "Т", "У", "Ф", "Х", "Ц", "Ш",
	"Щ", "Э", "Ю", "Я",
}

// Letter возвращает букву приложения по порядковому номеру (0-based).
// При исчерпании одиночных букв переходит к двойным: АА, АБ, ...
func Letter(n int) string {
	l := len(Letters)
	if n < l {
		return Letters[n]
	}
	n -= l
	return Letters[n/l] + Letters[n%l]
}

// CleanFileName убирает порядковый номер в начале имени файла.
// "5. Текстовая часть" → "Текстовая часть"
// "10. ПРИЛОЖЕНИЯ"    → "ПРИЛОЖЕНИЯ"
func CleanFileName(base string) string {
	for i, ch := range base {
		if ch == '.' && i > 0 {
			rest := strings.TrimSpace(base[i+1:])
			if rest != "" {
				return rest
			}
		}
	}
	return base
}

// MarkerAppendixBegin — подстрока имени заглушки начала приложений.
const MarkerAppendixBegin = "ПРИЛОЖЕНИ"

// MarkerAppendixEnd — подстрока имени заглушки конца приложений.
const MarkerAppendixEnd = "ГРАФИЧЕСК"

// IsMarker проверяет что имя содержит маркер (case-insensitive).
func IsMarker(name, marker string) bool {
	return strings.Contains(strings.ToUpper(name), marker)
}
