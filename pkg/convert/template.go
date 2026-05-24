//go:build windows

package convert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"log/slog"

	gotemplatedocx "github.com/JJJJJJack/go-template-docx"
	"github.com/Nemo08/ppdftb/pkg/fileutil"
)

// MediaLoader содержит картинки для подстановки в DOCX-шаблоны.
// Static — картинки из папки -p (ключ — имя файла, значение — []byte).
// Mapped — картинки, пути к которым указаны в XML-данных (ключ — имя, значение — []byte).
type MediaLoader struct {
	mu     sync.RWMutex
	Static map[string][]byte
	Mapped map[string][]byte
}

// RangeStatic безопасно перечисляет Static-картинки.
func (m *MediaLoader) RangeStatic(fn func(k string, v []byte) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, v := range m.Static {
		if !fn(k, v) {
			break
		}
	}
}

// RangeMapped безопасно перечисляет Mapped-картинки.
func (m *MediaLoader) RangeMapped(fn func(k string, v []byte) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, v := range m.Mapped {
		if !fn(k, v) {
			break
		}
	}
}

// LoadMedia загружает картинки из папки -p и из полей-путей в data.
func LoadMedia(picsDir string, data []byte) *MediaLoader {
	m := &MediaLoader{
		Static: make(map[string][]byte),
		Mapped: make(map[string][]byte),
	}
	loadStaticImages(picsDir, m)
	loadMappedImages(data, m)
	return m
}

func loadStaticImages(picsDir string, m *MediaLoader) {
	if picsDir == "" {
		return
	}
	entries, err := os.ReadDir(picsDir)
	if err != nil {
		slog.Default().Error("не удалось прочитать папку с картинками", slog.String("dir", picsDir), slog.String("err", err.Error()))
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
			continue
		}
		fullPath := filepath.Join(picsDir, entry.Name())
		imageContent, err := os.ReadFile(fullPath)
		if err != nil {
			slog.Default().Error("не удалось прочитать картинку", slog.String("file", fullPath), slog.String("err", err.Error()))
			continue
		}
		m.Static[entry.Name()] = imageContent
		slog.Default().Debug("загружена картинка из -p", slog.String("file", entry.Name()))
	}
}

func loadMappedImages(data []byte, m *MediaLoader) {
	if len(data) == 0 {
		return
	}
	var dataMap map[string]any
	if err := json.Unmarshal(data, &dataMap); err != nil {
		slog.Default().Error("разбор JSON для mapped-картинок", slog.String("err", err.Error()))
		return
	}
	for k, v := range dataMap {
		strVal, ok := v.(string)
		if !ok {
			continue
		}
		loadImageFromPath(k, strVal, m)
	}
}

func loadImageFromPath(key, val string, m *MediaLoader) {
	normalizedVal := strings.ReplaceAll(val, `\`, `/`)
	ext := strings.ToLower(path.Ext(normalizedVal))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		return
	}
	imageContent, err := os.ReadFile(val)
	if err != nil {
		slog.Default().Error("не удалось прочитать картинку", slog.String("file", val), slog.String("err", err.Error()))
		return
	}
	filename := path.Base(normalizedVal)
	m.Mapped[filename] = imageContent
	m.Mapped[val] = imageContent
	m.Mapped[normalizedVal] = imageContent
	slog.Default().Debug("загружена картинка из данных", slog.String("key", key), slog.String("file", filename))
}

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
// This is the shared core used by both TplToPdfWithPool and TplToDocx.
func processOneFile(ctx context.Context, fn, odn string, data []byte, media *MediaLoader) templateResult {
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
		gotemplatedocx.IgnoreMissingKey(),
	)
	if err != nil {
		return templateResult{fn: fn, err: fmt.Errorf("open template %s: %w", filepath.Base(fn), err)}
	}
	jtpl.AddTemplateFuncs(tmaps)
	media.RangeStatic(func(k string, v []byte) bool {
		jtpl.Media(k, v)
		return true
	})
	media.RangeMapped(func(k string, v []byte) bool {
		jtpl.Media(k, v)
		return true
	})

	if err := jtpl.Apply(data); err != nil {
		return templateResult{fn: fn, err: fmt.Errorf("apply %s: %w", filepath.Base(fn), err)}
	}

	docxPath := filepath.Join(odn, strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))+".docx")
	if err := jtpl.Save(docxPath); err != nil {
		return templateResult{fn: fn, err: fmt.Errorf("save %s: %w", filepath.Base(fn), err)}
	}

	return templateResult{fn: fn, docxPath: docxPath, isDocx: true}
}

// TplToPdfWithPool подставляет данные в DOCX-шаблоны и сразу конвертирует результат в PDF.
// Каждый файл обрабатывается полностью в своей горутине: шаблонизация → .docx → .pdf,
// без ожидания окончания шаблонизации всех файлов перед началом конвертации.
func TplToPdfWithPool(ctx context.Context, pool WordConverter, inputWordFiles []string, docxFolder, pdfFolder string, data []byte, picsDir string) error {
	odn, err := filepath.Abs(docxFolder)
	if err != nil {
		return err
	}
	pdn, err := filepath.Abs(pdfFolder)
	if err != nil {
		return err
	}

	media := LoadMedia(picsDir, data)

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

		res := processOneFile(ctx, fn, odn, data, media)
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
	odn, err := filepath.Abs(outputFolder)
	if err != nil {
		return err
	}

	media := LoadMedia(picsDir, data)

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

		res := processOneFile(ctx, fn, odn, data, media)
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
