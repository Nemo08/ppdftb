package pdfpl

import "github.com/Nemo08/ppdftb/pkg/fontutil"

// pkg/pdfpl — шаблонизатор: поиск и замена плейсхолдеров в контент-потоках.
// Всё, что связано со шрифтами (поиск системных TrueType, проверка покрытия
// рун, внедрение, кодекс рана), живёт в pkg/fontutil. Здесь остаются только
// алиасы типов и локальные обёртки, чтобы слои потоков и ранов не переписывать.

// RunCodec — пофайловое кодирование и измерение текста ран-блока при
// переключении рана на внедрённый шрифт.
type RunCodec = fontutil.RunCodec

// CodecProvider выбирает кодировщик для ран-блока.
type CodecProvider = fontutil.CodecProvider

// textWidth оценивает ширину строки в тысячных долях em: фолбэк для ранов без
// кодекса (оригинальный шрифт, в котором плейсхолдер нарисован).
func textWidth(s string) float64 { return fontutil.EstimateWidth(s) }
