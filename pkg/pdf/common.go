// Package pdf предоставляет утилиты для работы с PDF: объединение файлов,
// нумерация страниц, пересчёт единиц измерения.
package pdf

import (
	"os"
	"path/filepath"
	"strings"

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

// CollectPdfFiles читает директорию и возвращает список абсолютных путей к .pdf файлам.
// Используется как в merge, так и в toc.
func CollectPdfFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.ToLower(filepath.Ext(e.Name())) == ".pdf" {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files, nil
}

// PageCount возвращает количество страниц в PDF-файле.
// Используется в engine (readPageCount) и toc (getPdfPageCount).
func PageCount(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	pr, err := pdf.NewPdfReader(f)
	if err != nil {
		return 0, err
	}
	return pr.GetNumPages()
}
