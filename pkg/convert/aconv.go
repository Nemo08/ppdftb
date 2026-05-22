//go:build windows

package convert

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"log/slog"

	acadpool "github.com/Nemo08/ppdftb/pkg/acadpool"
	pdf "github.com/Nemo08/ppdftb/pkg/pdf"
)

// A2pdf конвертирует DWG/DXF файлы в PDF через пул AutoCAD.
func A2pdf(ctx context.Context, sourceFile, sourceFolder, outputFolder string) error {
	inputCadFiles := CollectCadFiles(sourceFile, sourceFolder)
	if len(inputCadFiles) == 0 {
		slog.InfoContext(ctx, "Нет DWG/DXF файлов для конвертации")
		return nil
	}

	pool := acadpool.NewAcadPool(1)
	defer pool.Close()
	return A2pdfWithPool(ctx, pool, inputCadFiles, outputFolder)
}

// A2pdfWithPool делает то же, что A2pdf, но использует переданный AcadPool.
func A2pdfWithPool(ctx context.Context, pool *acadpool.AcadPool, files []string, outputFolder string) error {
	var wg sync.WaitGroup
	for _, file := range files {
		wg.Add(1)
		go func(f string) {
			defer wg.Done()

			outDir, err := os.MkdirTemp("", "aconv-")
			if err != nil {
				slog.ErrorContext(ctx, err.Error())
				return
			}
			defer os.RemoveAll(outDir)

			err = pool.AcadToPdf(ctx, f, outDir)
			if err != nil {
				slog.ErrorContext(ctx, err.Error())
				return
			}

			cleanName := strings.TrimSuffix(f, filepath.Ext(f))
			slog.Debug("merge output", slog.String("file", filepath.Join(outputFolder, filepath.Base(cleanName)+".pdf")))

			err = pdf.Merge(ctx, outDir, filepath.Join(outputFolder, filepath.Base(cleanName)+".pdf"))
			if err != nil {
				slog.ErrorContext(ctx, err.Error())
			}
		}(file)
	}
	wg.Wait()
	return nil
}

// CollectCadFiles собирает DWG/DXF из файла или папки.
func CollectCadFiles(sourceFile, sourceFolder string) []string {
	var sources []string
	if sourceFile != "" {
		sources = append(sources, sourceFile)
	}
	if sourceFolder != "" {
		sources = append(sources, sourceFolder)
	}
	if len(sources) == 0 {
		return nil
	}
	files, err := CollectFiles(sources, []string{".dwg", ".dxf"})
	if err != nil {
		slog.Error(err.Error())
		return nil
	}
	return files
}
