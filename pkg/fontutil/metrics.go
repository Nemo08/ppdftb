package fontutil

import model "github.com/oliverpool/unipdf/v3/model"

// Точные метрики шрифта — в отличие от EstimateWidth (estimate.go), которая
// считает ширину эвристикой по классам символов. Здесь берутся настоящие
// авансы глифов из внедрённого шрифта, поэтому результат годится там, где
// ошибка ширины видна в файле: расчёт габарита блока, выравнивание по правому
// краю, подгонка отступа.
//
// EstimateWidth остаётся там, где шрифта нет вовсе: фолбэк для ранов без
// кодекса (см. pdfpl.textWidth), когда текст нарисован оригинальным шрифтом
// плейсхолдера и его метрики недоступны.

// defaultAdvanceEm — ширина неизвестной руны в тысячных долях em (половина em).
// Тот же фолбэк, что у EstimateWidth: значение из type1 AFM для Helvetica.
const defaultAdvanceEm = 500.0

// Advance возвращает натуральную ширину руны в пунктах при кегле size.
//
// Тонкость unipdf: PdfFont.GetRuneMetrics для руны, которой нет в шрифте,
// НЕ возвращает ok=false, а отдаёт /MissingWidth из FontDescriptor с
// ok=true (model/font.go:593). Поэтому проверять надо и признак, и сам
// /MissingWidth: уMany-шрифтов он равен 0, и без проверки блок схлопнется
// в точку. Нулевая ширина — всегда дефект, поэтому подменяем её типичной.
func Advance(f *model.PdfFont, r rune, size float64) float64 {
	if f != nil {
		if cm, ok := f.GetRuneMetrics(r); ok && cm.Wx > 0 {
			return cm.Wx * size / 1000
		}
	}
	return defaultAdvanceEm * size / 1000
}

// StringWidth возвращает суммарную ширину строки в пунктах при кегле size.
// Кернинга не учитывает: для блочной вёрстки (габарит блока, отступ до поля)
// сумма авансов достаточна, а разница на одну-две руны в пределах допуска.
func StringWidth(f *model.PdfFont, s string, size float64) float64 {
	var w float64
	for _, r := range s {
		w += Advance(f, r, size)
	}
	return w
}
