//go:build windows

// Package convert содержит логику конвертации шаблонов и подстановок:
// DOCX/DWG в PDF через COM-пулы Word/AutoCAD.
package convert

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"log/slog"
)

// AconvOptions настраивает параметры конвертации DWG/DXF в PDF.
type AconvOptions struct {
	TempPrefix string
}

// A2pdfWithPool конвертирует DWG/DXF файлы в PDF через переданный CadConverter
// и сливает результат через PdfMerger. Использует общую временную папку для всех файлов.
func A2pdfWithPool(ctx context.Context, pool CadConverter, merger PdfMerger, files []string, outputFolder string) error {
	return A2pdfWithPoolCfg(ctx, pool, merger, files, outputFolder, AconvOptions{TempPrefix: "aconv-"})
}

// A2pdfWithPoolCfg — то же, что A2pdfWithPool, но с группировкой опций в AconvOptions.
func A2pdfWithPoolCfg(ctx context.Context, pool CadConverter, merger PdfMerger, files []string, outputFolder string, opts AconvOptions) error {
	tempPrefix := opts.TempPrefix
	if tempPrefix == "" {
		tempPrefix = "aconv-"
	}
	baseDir, err := os.MkdirTemp("", tempPrefix)
	if err != nil {
		return err
	}
	defer func() {
		if err := os.RemoveAll(baseDir); err != nil {
			slog.Warn("очистка временного каталога A2pdf", slog.String("dir", baseDir), slog.String("err", err.Error()))
		}
	}()

	errCh := make(chan error, len(files))
	var wg sync.WaitGroup
	for _, file := range files {
		wg.Add(1)
		go func(f string) {
			defer wg.Done()

			outDir, err := os.MkdirTemp(baseDir, filepath.Base(f)+"-")
			if err != nil {
				slog.ErrorContext(ctx, err.Error())
				errCh <- err
				return
			}

			err = pool.AcadToPdf(ctx, f, outDir)
			if err != nil {
				slog.ErrorContext(ctx, err.Error())
				errCh <- fmt.Errorf("%s: %w", filepath.Base(f), err)
				return
			}

			cleanName := strings.TrimSuffix(f, filepath.Ext(f))
			slog.Debug("merge output", slog.String("file", filepath.Join(outputFolder, filepath.Base(cleanName)+".pdf")))

			err = merger.Merge(ctx, outDir, filepath.Join(outputFolder, filepath.Base(cleanName)+".pdf"))
			if err != nil {
				slog.ErrorContext(ctx, err.Error())
				errCh <- fmt.Errorf("merge %s: %w", filepath.Base(cleanName), err)
			}
		}(file)
	}
	wg.Wait()
	close(errCh)

	var errs []error
	for e := range errCh {
		errs = append(errs, e)
	}
	return errors.Join(errs...)
}
