package convert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

type Map map[string]string

// FilesToPdf принимает список файлов или папок, собирает из них *.doc/*.docx/*.rtf
// и конвертирует каждый в PDF через пул Word, складывая результат в outputFolder.
func FilesToPdf(ctx context.Context, sources []string, outputFolder string) error {
	odn, err := filepath.Abs(outputFolder)
	if err != nil {
		slog.Default().ErrorContext(ctx, err.Error())
		return err
	}

	// Раскрываем папки в список файлов.
	var inputWordFiles []string
	for _, src := range sources {
		info, err := os.Stat(src)
		if err != nil {
			slog.Default().ErrorContext(ctx, "недоступен источник", slog.String("src", src), slog.String("err", err.Error()))
			continue
		}
		if info.IsDir() {
			// Собираем все подходящие файлы из папки.
			entries, err := os.ReadDir(src)
			if err != nil {
				slog.Default().ErrorContext(ctx, err.Error())
				return err
			}
			for _, e := range entries {
				if e.IsDir() || strings.HasPrefix(e.Name(), "~$") {
					continue
				}
				ext := strings.ToLower(filepath.Ext(e.Name()))
				if ext == ".doc" || ext == ".docx" || ext == ".rtf" {
					abs, err := filepath.Abs(filepath.Join(src, e.Name()))
					if err != nil {
						return err
					}
					inputWordFiles = append(inputWordFiles, abs)
				}
			}
		} else {
			// Это конкретный файл.
			ext := strings.ToLower(filepath.Ext(src))
			if ext == ".doc" || ext == ".docx" || ext == ".rtf" {
				abs, err := filepath.Abs(src)
				if err != nil {
					return err
				}
				inputWordFiles = append(inputWordFiles, abs)
			}
		}
	}

	if len(inputWordFiles) == 0 {
		slog.Default().DebugContext(ctx, "нет файлов для конвертации")
		return nil
	}

	slog.Default().DebugContext(ctx, "файлов к конвертации в PDF", slog.Int("count", len(inputWordFiles)))

	pool := NewWordPool(4)
	defer pool.Close()

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

func makeTfm() map[string]any {
	return map[string]any{
		"add":      func(a, b int) int { return a + b },
		"year":     func() string { return strconv.Itoa(time.Now().Year()) },
		"nowdate":  func() string { return time.Now().Format("02.01.2006") },
		"datetime": func() string { return time.Now().Format("02.01.2006 15:04") },
	}
}

func TplToDocxJJack3(ctx context.Context, inputWordFiles []string, outputFolder string, data []byte, picsDir string) error {
	var err error

	odn, err := filepath.Abs(outputFolder) //Полный путь выходной папки
	if err != nil {
		slog.Default().ErrorContext(ctx, err.Error())
		return err
	}

	// Картинки из папки -p: ключ = имя файла (например "logo.png")
	var staticMedia sync.Map
	if picsDir != "" {
		entries, err := os.ReadDir(picsDir)
		if err != nil {
			slog.Default().ErrorContext(ctx, fmt.Errorf("не удалось прочитать папку с картинками %s: %w", picsDir, err).Error())
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
					slog.Default().ErrorContext(ctx, fmt.Errorf("не удалось прочитать картинку %s: %w", fullPath, err).Error())
					continue
				}
				staticMedia.Store(entry.Name(), imageContent)
				slog.Default().DebugContext(ctx, "загружена картинка из -p", slog.String("file", entry.Name()))
			}
		}
	}

	// Картинки из данных: значения в data которые являются путями к jpg/png.
	// Регистрируются дважды — по имени файла и по ключу поля.
	// Поддерживаются Windows UNC-пути вида \\server\share\file.png с обратными слэшами.
	var mappedMedia sync.Map
	if len(data) > 0 {
		var dataMap map[string]any
		if err := json.Unmarshal(data, &dataMap); err == nil {
			for k, v := range dataMap {
				strVal, ok := v.(string)
				if !ok {
					continue
				}
				// Нормализуем обратные слэши в прямые для корректного определения расширения.
				// filepath.Ext на Linux не понимает пути с \\ как разделители.
				normalizedVal := strings.ReplaceAll(strVal, `\`, `/`)
				ext := strings.ToLower(path.Ext(normalizedVal))
				if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
					continue
				}
				// Пробуем оригинальный путь (Windows UNC работает на Windows)
				imageContent, err := os.ReadFile(strVal)
				if err != nil {
					slog.Default().ErrorContext(ctx, fmt.Errorf("не удалось прочитать картинку %s: %w", strVal, err).Error())
					continue
				}
				// Имя файла извлекаем тоже через нормализованный путь
				filename := path.Base(normalizedVal)
				// Регистрируем под тремя ключами — replaceImages() ищет в mediaMap
				// по значению поля как есть (полный путь), а не по Base().
				// Media() делает filepath.Base() внутри, поэтому передаём нормализованный
				// путь (с /) чтобы Base() дал правильное имя файла.
				mappedMedia.Store(filename, imageContent)      // "Магистраль.png"
				mappedMedia.Store(strVal, imageContent)        // "\\\\server\\path\\Магистраль.png"
				mappedMedia.Store(normalizedVal, imageContent) // "//server/path/Магистраль.png"
				slog.Default().DebugContext(ctx, "загружена картинка из данных", slog.String("key", k), slog.String("file", filename))
			}
		}
	}

	var wg sync.WaitGroup
	wg.Add(len(inputWordFiles))

	//Заполняем шаблоны
	work := func(fn string, data []byte) {
		// Создаём новый экземпляр FuncMap для каждой горутины.
		tmaps := makeTfm()

		defer wg.Done()
		// Защита от паник в библиотеке go-template-docx
		// (extractFieldNamesRec падает с nil pointer при обходе range-узлов с массивами).
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

		// WarnOnMissingKey не используем: его extractFieldNamesRec падает с nil pointer
		// когда данные содержат массивы (range-узлы в шаблоне).
		jtpl, err := gotemplatedocx.NewDocxTemplateFromFilename(fn, gotemplatedocx.NoRemoveEmptyTableRows(), gotemplatedocx.RemoveRangeRows(), gotemplatedocx.IgnoreMissingKey())
		if err != nil {
			slog.Default().ErrorContext(ctx, fmt.Errorf("template error 2 %w in file %s", err, fn).Error())
			return
		}
		//доп функции
		jtpl.AddTemplateFuncs(tmaps)

		// Грузим картинки из папки -p (по имени файла)
		staticMedia.Range(func(key, value interface{}) bool {
			jtpl.Media(key.(string), value.([]byte))
			return true
		})

		// Грузим картинки из данных (по имени файла и по ключу поля)
		mappedMedia.Range(func(key, value interface{}) bool {
			jtpl.Media(key.(string), value.([]byte))
			return true
		})

		// Apply нужно вызывать всегда — без него output буфер остаётся пустым и Save запишет файл нулевого размера.
		// Пустой data корректно обрабатывается внутри Apply (шаблон рендерится без подстановок).
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

func filecopy(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return 0, errors.New("error copy of file " + src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}

// CollectWordFiles собирает все *.doc/*.docx/*.rtf из списка файлов и папок.
// Используется при принудительной конвертации (-f) минуя кэш.
func CollectWordFiles(sources []string) ([]string, error) {
	var result []string
	for _, src := range sources {
		info, err := os.Stat(src)
		if err != nil {
			return nil, fmt.Errorf("недоступен источник %q: %w", src, err)
		}
		if info.IsDir() {
			entries, err := os.ReadDir(src)
			if err != nil {
				return nil, err
			}
			for _, e := range entries {
				if e.IsDir() || strings.HasPrefix(e.Name(), "~$") {
					continue
				}
				ext := strings.ToLower(filepath.Ext(e.Name()))
				if ext == ".doc" || ext == ".docx" || ext == ".rtf" {
					abs, err := filepath.Abs(filepath.Join(src, e.Name()))
					if err != nil {
						return nil, err
					}
					result = append(result, abs)
				}
			}
		} else {
			ext := strings.ToLower(filepath.Ext(src))
			if ext == ".doc" || ext == ".docx" || ext == ".rtf" {
				abs, err := filepath.Abs(src)
				if err != nil {
					return nil, err
				}
				result = append(result, abs)
			}
		}
	}
	return result, nil
}
