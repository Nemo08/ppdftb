//go:build windows

package convert

import (
	"context"
	"encoding/json"
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
	if picsDir != "" {
		entries, err := os.ReadDir(picsDir)
		if err != nil {
			slog.Default().Error("не удалось прочитать папку с картинками", slog.String("dir", picsDir), slog.String("err", err.Error()))
		} else {
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
	}
	if len(data) > 0 {
		var dataMap map[string]any
		if err := json.Unmarshal(data, &dataMap); err == nil {
			for k, v := range dataMap {
				strVal, ok := v.(string)
				if !ok {
					continue
				}
				normalizedVal := strings.ReplaceAll(strVal, `\`, `/`)
				ext := strings.ToLower(path.Ext(normalizedVal))
				if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
					continue
				}
				imageContent, err := os.ReadFile(strVal)
				if err != nil {
					slog.Default().Error("не удалось прочитать картинку", slog.String("file", strVal), slog.String("err", err.Error()))
					continue
				}
				filename := path.Base(normalizedVal)
				m.Mapped[filename] = imageContent
				m.Mapped[strVal] = imageContent
				m.Mapped[normalizedVal] = imageContent
				slog.Default().Debug("загружена картинка из данных", slog.String("key", k), slog.String("file", filename))
			}
		}
	}
	return m
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

	var wg sync.WaitGroup
	for _, file := range inputWordFiles {
		wg.Add(1)
		go func(f string) {
			defer wg.Done()
			slog.Default().DebugContext(ctx, "Конвертируем файл", slog.String("file", filepath.Base(f)))
			out := filepath.Join(odn, strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))+".pdf")
			if err := pool.WordToPdf(ctx, f, out); err != nil {
				slog.Default().ErrorContext(ctx, "конвертация", slog.String("file", f), slog.String("err", err.Error()))
			}
		}(file)
	}
	wg.Wait()
	return nil
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

	var wg sync.WaitGroup
	wg.Add(len(inputWordFiles))

	work := func(fn string, data []byte) {
		tmaps := tplFuncs()

		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Default().ErrorContext(ctx, fmt.Sprintf("panic в шаблонизаторе файл=%s: %v", filepath.Base(fn), r))
			}
		}()

		if strings.ToLower(filepath.Ext(fn)) != ".docx" {
			_, err := filecopy(fn, filepath.Join(odn, strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))+strings.ToLower(filepath.Ext(fn))))
			if err != nil {
				slog.Default().ErrorContext(ctx, fmt.Errorf("template error 1 %w in file %s", err, fn).Error())
			}
			return
		}

		slog.Default().DebugContext(ctx, "Конвертируем файл", slog.String("file", filepath.Base(fn)))

		jtpl, err := gotemplatedocx.NewDocxTemplateFromFilename(fn, gotemplatedocx.NoRemoveEmptyTableRows(), gotemplatedocx.RemoveRangeRows(), gotemplatedocx.IgnoreMissingKey())
		if err != nil {
			slog.Default().ErrorContext(ctx, fmt.Errorf("template error 2 %w in file %s", err, fn).Error())
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
			return
		}

		err = jtpl.Save(filepath.Join(odn, strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))+".docx"))
		if err != nil {
			slog.Default().ErrorContext(ctx, fmt.Errorf("template error 4 %w in file %s", err, fn).Error())
			return
		}
	}

	for _, f := range inputWordFiles {
		go work(f, data)
	}
	wg.Wait()
	return nil
}


