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

	inputWordFiles, err := CollectWordFiles(sources)
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

	work := func(fn string, data []byte) {
		tmaps := tplFuncs()

		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				msg := fmt.Sprintf("panic в шаблонизаторе файл=%s: %v", filepath.Base(fn), r)
				slog.Default().ErrorContext(ctx, msg)
				errCh <- fmt.Errorf("%s", msg)
			}
		}()

		if strings.ToLower(filepath.Ext(fn)) != ".docx" {
			_, err := filecopy(fn, filepath.Join(odn, strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))+strings.ToLower(filepath.Ext(fn))))
			if err != nil {
				slog.Default().ErrorContext(ctx, fmt.Errorf("template error 1 %w in file %s", err, fn).Error())
				errCh <- fmt.Errorf("copy %s: %w", filepath.Base(fn), err)
			}
			return
		}

		slog.Default().DebugContext(ctx, "Шаблонизируем файл", slog.String("file", filepath.Base(fn)))

		jtpl, err := gotemplatedocx.NewDocxTemplateFromFilename(fn, gotemplatedocx.NoRemoveEmptyTableRows(), gotemplatedocx.RemoveRangeRows(), gotemplatedocx.IgnoreMissingKey())
		if err != nil {
			slog.Default().ErrorContext(ctx, fmt.Errorf("template error 2 %w in file %s", err, fn).Error())
			errCh <- fmt.Errorf("open template %s: %w", filepath.Base(fn), err)
			return
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

		err = jtpl.Apply(data)
		if err != nil {
			slog.Default().ErrorContext(ctx, fmt.Errorf("template error 3 %w in file %s", err, fn).Error())
			errCh <- fmt.Errorf("apply %s: %w", filepath.Base(fn), err)
			return
		}

		docxPath := filepath.Join(odn, strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))+".docx")
		err = jtpl.Save(docxPath)
		if err != nil {
			slog.Default().ErrorContext(ctx, fmt.Errorf("template error 4 %w in file %s", err, fn).Error())
			errCh <- fmt.Errorf("save %s: %w", filepath.Base(fn), err)
			return
		}

		pdfPath := filepath.Join(pdn, strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))+".pdf")
		if err := pool.WordToPdf(ctx, docxPath, pdfPath); err != nil {
			slog.Default().ErrorContext(ctx, "конвертация в PDF", slog.String("file", filepath.Base(fn)), slog.String("err", err.Error()))
			errCh <- fmt.Errorf("%s: %w", filepath.Base(fn), err)
			return
		}
	}

	for _, f := range inputWordFiles {
		wg.Add(1)
		go work(f, data)
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

// TplToDocxJJack3 подставляет данные в DOCX-шаблоны и сохраняет результат.
// inputWordFiles — пути к шаблонам .docx, outputFolder — куда сохранять готовые документы.
// data — XML/JSON с данными для подстановки, picsDir — папка с картинками (Media).
func TplToDocxJJack3(ctx context.Context, inputWordFiles []string, outputFolder string, data []byte, picsDir string) error {
	odn, err := filepath.Abs(outputFolder)
	if err != nil {
		slog.Default().ErrorContext(ctx, err.Error())
		return err
	}

	media := LoadMedia(picsDir, data)

	errCh := make(chan error, len(inputWordFiles))
	var wg sync.WaitGroup
	wg.Add(len(inputWordFiles))

	work := func(fn string, data []byte) {
		tmaps := tplFuncs()

		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				msg := fmt.Sprintf("panic в шаблонизаторе файл=%s: %v", filepath.Base(fn), r)
				slog.Default().ErrorContext(ctx, msg)
				errCh <- fmt.Errorf("%s", msg)
			}
		}()

		if strings.ToLower(filepath.Ext(fn)) != ".docx" {
			_, err := filecopy(fn, filepath.Join(odn, strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))+strings.ToLower(filepath.Ext(fn))))
			if err != nil {
				slog.Default().ErrorContext(ctx, fmt.Errorf("template error 1 %w in file %s", err, fn).Error())
				errCh <- fmt.Errorf("copy %s: %w", filepath.Base(fn), err)
			}
			return
		}

		slog.Default().DebugContext(ctx, "Конвертируем файл", slog.String("file", filepath.Base(fn)))

		jtpl, err := gotemplatedocx.NewDocxTemplateFromFilename(fn, gotemplatedocx.NoRemoveEmptyTableRows(), gotemplatedocx.RemoveRangeRows(), gotemplatedocx.IgnoreMissingKey())
		if err != nil {
			slog.Default().ErrorContext(ctx, fmt.Errorf("template error 2 %w in file %s", err, fn).Error())
			errCh <- fmt.Errorf("open template %s: %w", filepath.Base(fn), err)
			return
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

		err = jtpl.Apply(data)
		if err != nil {
			slog.Default().ErrorContext(ctx, fmt.Errorf("template error 3 %w in file %s", err, fn).Error())
			errCh <- fmt.Errorf("apply %s: %w", filepath.Base(fn), err)
			return
		}

		err = jtpl.Save(filepath.Join(odn, strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))+".docx"))
		if err != nil {
			slog.Default().ErrorContext(ctx, fmt.Errorf("template error 4 %w in file %s", err, fn).Error())
			errCh <- fmt.Errorf("save %s: %w", filepath.Base(fn), err)
			return
		}
	}

	for _, f := range inputWordFiles {
		go work(f, data)
	}
	wg.Wait()
	close(errCh)

	var errs []error
	for e := range errCh {
		errs = append(errs, e)
	}
	return errors.Join(errs...)
}
