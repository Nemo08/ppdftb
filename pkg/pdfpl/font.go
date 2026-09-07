package pdfpl

// glyphWidth возвращает ширину символа в тысячных долях em (glyph units).
// Используется только для расчёта дельты кернинга при замене плейсхолдера
// на значение той же ширины — точные метрики шрифта из файла не читаем.
// Для неизвестных символов берётся типичная ширина 500 (половина em).
func glyphWidth(r rune) float64 {
	switch {
	case r == 'I' || r == '1' || r == ',' || r == '.' || r == ';' || r == ':':
		// Узкие символы проверяются до диапазонов: '1' входит и в
		// '0'..'9', а 'I' — в 'A'..'Z'.
		return 278
	case r >= '0' && r <= '9':
		return 500
	case r >= 'A' && r <= 'Z':
		return 650
	case r >= 'a' && r <= 'z':
		return 500
	case r == ' ' || r == '\u00a0':
		return 278
	default:
		return 500
	}
}

// textWidth оценивает ширину строки в тысячных долях em.
func textWidth(s string) float64 {
	var w float64
	for _, r := range s {
		w += glyphWidth(r)
	}
	return w
}
