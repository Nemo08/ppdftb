//go:build windows

package convert

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"log/slog"
)

// A2pdfWithPool конвертирует DWG/DXF файлы в PDF через переданный CadConverter
// и сливает результат через PdfMerger.
func A2pdfWithPool(ctx context.Context, pool CadConverter, merger PdfMerger, files []string, outputFolder string) error {
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

			err = merger.Merge(ctx, outDir, filepath.Join(outputFolder, filepath.Base(cleanName)+".pdf"))
			if err != nil {
				slog.ErrorContext(ctx, err.Error())
			}
		}(file)
	}
	wg.Wait()
	return nil
}
