//go:build windows

package convert

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"log/slog"

	gotemplatedocx "github.com/JJJJJJack/go-template-docx"
	"github.com/Nemo08/ppdftb/pkg/fileutil"
	"github.com/Nemo08/ppdftb/pkg/media"
)

// FilesToPdfWithPool конвертирует Word-файлы в PDF через переданный WordConverter.
func FilesToPdfWithPool(ctx context.Context, pool WordConverter, sources []string, outputFolder string) error {
	odn, err := filepath.Abs(outputFolder)
	if err != nil {
		return err
	}

	inputWordFiles, err := fileutil.CollectWordFiles(sources)
	if err != nil {
		return err
	}
	if len(inputWordFiles) == 0 {
		slog.Default().DebugContext(ctx, "нет файлов для конвертации")
		return nil
	}

	slog.Default().DebugContext(ctx, "файлов к конвертации в PDF", slog.Int("count", len(inputWordFiles)))

	errCh := make(chan error, len(inputWordFiles))
	var wg sync.WaitGroup
	for _, file := range inputWordFiles {
		wg.Add(1)
		go func(f string) {
			defer wg.Done()
			slog.Default().DebugContext(ctx, "Конвертируем файл", slog.String("file", filepath.Base(f)))
			out := filepath.Join(odn, strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))+".pdf")
			if err := pool.WordToPdf(ctx, f, out); err != nil {
				slog.Default().ErrorContext(ctx, "конвертация", slog.String("file", f), slog.String("err", err.Error()))
				errCh <- fmt.Errorf("%s: %w", filepath.Base(f), err)
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

// templateResult describes the outcome of processing one template file.
// Used internally by TplToPdfWithPool and TplToDocx to share logic.
type templateResult struct {
	fn       string
	docxPath string // path to generated .docx (empty for non-docx)
	isDocx   bool
	err      error
}

// processOneFile handles template substitution (or copy) for a single input file.
// For .docx files it applies the template and saves to odn; for other files it copies as-is.
//
// Картинки (Media) и текстовые поля — один проход Apply: Media должен быть
// зарегистрирован ДО Apply, иначе image-плейсхолдеры не заполняются.
//
// Разворачивание {{Pages.Name}} (docxplate AutoExpandRows) сюда НЕ входит:
// это RenderOption и ответственность toc.Make. Render без Media перед Apply
// съедает/ломает image-плейсхолдеры — так делать нельзя.
func processOneFile(ctx context.Context, fn, odn string, data []byte, m *media.MediaLoader) templateResult {
	tmaps := tplFuncs()

	if strings.ToLower(filepath.Ext(fn)) != ".docx" {
		dst := filepath.Join(odn, strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))+strings.ToLower(filepath.Ext(fn)))
		_, err := filecopy(fn, dst)
		if err != nil {
			return templateResult{fn: fn, err: fmt.Errorf("copy %s: %w", filepath.Base(fn), err)}
		}
		return templateResult{fn: fn}
	}

	slog.Default().DebugContext(ctx, "Шаблонизируем файл", slog.String("file", filepath.Base(fn)))

	jtpl, err := gotemplatedocx.NewDocxTemplateFromFilename(fn,
		gotemplatedocx.NoRemoveEmptyTableRows(),
		gotemplatedocx.RemoveRangeRows(),
		gotemplatedocx.DeleteMissingKey(),
		gotemplatedocx.IgnoreMissingKey(),
		gotemplatedocx.DocxplateCompat(),
	)
	if err != nil {
		return templateResult{fn: fn, err: fmt.Errorf("open template %s: %w", filepath.Base(fn), err)}
	}
	jtpl.AddTemplateFuncs(tmaps)

	nMedia := 0
	m.RangeStatic(func(k string, v []byte) bool {
		jtpl.Media(k, v)
		nMedia++
		return true
	})
	m.RangeMapped(func(k string, v []byte) bool {
		jtpl.Media(k, v)
		nMedia++
		return true
	})
	slog.Default().DebugContext(ctx, "Media зарегистрированы",
		slog.String("file", filepath.Base(fn)), slog.Int("count", nMedia))

	if err := jtpl.Apply(data); err != nil {
		return templateResult{fn: fn, err: fmt.Errorf("apply %s: %w", filepath.Base(fn), err)}
	}

	docxPath := filepath.Join(odn, strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))+".docx")
	if err := jtpl.Save(docxPath); err != nil {
		return templateResult{fn: fn, err: fmt.Errorf("save %s: %w", filepath.Base(fn), err)}
	}

	return templateResult{fn: fn, docxPath: docxPath, isDocx: true}
}

// TplOptions настраивает шаблонизацию: данные для подстановки и папка с картинками.
type TplOptions struct {
	Data    []byte
	PicsDir string
}

// TplToPdfWithPool подставляет данные в DOCX-шаблоны и сразу конвертирует результат в PDF.
// Каждый файл обрабатывается полностью в своей горутине: шаблонизация → .docx → .pdf,
// без ожидания окончания шаблонизации всех файлов перед началом конвертации.
func TplToPdfWithPool(ctx context.Context, pool WordConverter, inputWordFiles []string, docxFolder, pdfFolder string, data []byte, picsDir string) error {
	return TplToPdfWithPoolCfg(ctx, pool, inputWordFiles, docxFolder, pdfFolder, TplOptions{Data: data, PicsDir: picsDir})
}

// TplToPdfWithPoolCfg — то же, что TplToPdfWithPool, но с группировкой опций в TplOptions.
func TplToPdfWithPoolCfg(ctx context.Context, pool WordConverter, inputWordFiles []string, docxFolder, pdfFolder string, opts TplOptions) error {
	odn, err := filepath.Abs(docxFolder)
	if err != nil {
		return err
	}
	pdn, err := filepath.Abs(pdfFolder)
	if err != nil {
		return err
	}

	ml := media.LoadMedia(opts.PicsDir, opts.Data)

	errCh := make(chan error, len(inputWordFiles))
	var wg sync.WaitGroup

	work := func(fn string) {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				msg := fmt.Sprintf("panic в шаблонизаторе файл=%s: %v", filepath.Base(fn), r)
				slog.Default().ErrorContext(ctx, msg)
				errCh <- fmt.Errorf("%s", msg)
			}
		}()

		res := processOneFile(ctx, fn, odn, opts.Data, ml)
		if res.err != nil {
			errCh <- res.err
			return
		}
		if !res.isDocx {
			return
		}

		pdfPath := filepath.Join(pdn, strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))+".pdf")
		if err := pool.WordToPdf(ctx, res.docxPath, pdfPath); err != nil {
			errCh <- fmt.Errorf("%s: %w", filepath.Base(fn), err)
		}
	}

	for _, f := range inputWordFiles {
		wg.Add(1)
		go work(f)
	}
	wg.Wait()
	close(errCh)

	var errs []error
	for e := range errCh {
		errs = append(errs, e)
	}
	return errors.Join(errs...)
}

var tplFuncs = sync.OnceValue(func() map[string]any {
	return map[string]any{
		"add":      func(a, b int) int { return a + b },
		"year":     func() string { return strconv.Itoa(time.Now().Year()) },
		"nowdate":  func() string { return time.Now().Format("02.01.2006") },
		"datetime": func() string { return time.Now().Format("02.01.2006 15:04") },
	}
})

// TplToDocx подставляет данные в DOCX-шаблоны и сохраняет результат.
// inputWordFiles — пути к шаблонам .docx, outputFolder — куда сохранять готовые документы.
// data — XML/JSON с данными для подстановки, picsDir — папка с картинками (Media).
func TplToDocx(ctx context.Context, inputWordFiles []string, outputFolder string, data []byte, picsDir string) error {
	return TplToDocxCfg(ctx, inputWordFiles, outputFolder, TplOptions{Data: data, PicsDir: picsDir})
}

// TplToDocxCfg — то же, что TplToDocx, но с группировкой опций в TplOptions.
func TplToDocxCfg(ctx context.Context, inputWordFiles []string, outputFolder string, opts TplOptions) error {
	odn, err := filepath.Abs(outputFolder)
	if err != nil {
		return err
	}

	ml := media.LoadMedia(opts.PicsDir, opts.Data)

	errCh := make(chan error, len(inputWordFiles))
	var wg sync.WaitGroup

	work := func(fn string) {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				msg := fmt.Sprintf("panic в шаблонизаторе файл=%s: %v", filepath.Base(fn), r)
				slog.Default().ErrorContext(ctx, msg)
				errCh <- fmt.Errorf("%s", msg)
			}
		}()

		res := processOneFile(ctx, fn, odn, opts.Data, ml)
		if res.err != nil {
			errCh <- res.err
		}
	}

	for _, f := range inputWordFiles {
		wg.Add(1)
		go work(f)
	}
	wg.Wait()
	close(errCh)

	var errs []error
	for e := range errCh {
		errs = append(errs, e)
	}
	return errors.Join(errs...)
}
