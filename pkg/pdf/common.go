// Package pdf предоставляет утилиты для работы с PDF: объединение файлов,
// нумерация страниц, пересчёт единиц измерения.
package pdf

import (
	"log/slog"
	"os"

	pdf "github.com/oliverpool/unipdf/v3/model"
)

// Px2mm конвертирует пункты (pt) в миллиметры.
// 1 pt = 0.3528 мм (стандартный полиграфический пункт).
func Px2mm(px float64) float64 {
	return px * 0.3528
}

// Mm2px конвертирует миллиметры в пункты.
func Mm2px(mm float64) float64 {
	return mm / 0.3528
}

// PageCount возвращает количество страниц в PDF-файле.
// Используется в engine (readPageCount) и toc (getPdfPageCount).
func PageCount(path string) (int, error) {
	//nolint:gosec // чтение PDF по пути из аргументов CLI — ожидаемое поведение
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("close file", slog.String("path", path), slog.String("error", err.Error()))
		}
	}()

	pr, err := pdf.NewPdfReader(f)
	if err != nil {
		return 0, err
	}
	return pr.GetNumPages()
}
