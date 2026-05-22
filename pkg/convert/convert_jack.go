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

func makeTfm() map[string]any {
	return map[string]any{
		"add":      func(a, b int) int { return a + b },
		"year":     func() string { return strconv.Itoa(time.Now().Year()) },
		"nowdate":  func() string { return time.Now().Format("02.01.2006") },
		"datetime": func() string { return time.Now().Format("02.01.2006 15:04") },
	}
}

func TplToDocxJJack(ctx context.Context, source []string, outputFolder string, data map[string]string) error {
	var inputWordFiles []string
	var err error

	for _, v := range source {
		//Проверка наличия
		if stat, err := os.Stat(v); os.IsNotExist(err) {
			slog.Default().ErrorContext(ctx, v+" не существует")
		} else {
			if !strings.HasPrefix(stat.Name(), "~$") {
				var allFiles []string

				//Собираем список всех файлов и файлов в папках
				if stat.IsDir() {
					files, err := os.ReadDir(v)
					if err != nil {
						slog.Default().ErrorContext(ctx, err.Error())
						return err
					}
					for _, file := range files {
						if !file.IsDir() {
							allFiles = append(allFiles, filepath.Join(v, file.Name()))
						}
					}
				} else {
					allFiles = append(allFiles, v)
				}

				//Фильтруем список файлов
				for _, v := range allFiles {
					if !strings.HasPrefix(filepath.Base(v), "~$") &&
						(strings.ToLower(filepath.Ext(v)) == ".doc" ||
							strings.ToLower(filepath.Ext(v)) == ".docx" ||
							strings.ToLower(filepath.Ext(v)) == ".rtf") {
						fullFileName, err := filepath.Abs(v) //Полный путь входного файла
						if err != nil {
							slog.Default().ErrorContext(ctx, err.Error())
							return err
						}

						inputWordFiles = append(inputWordFiles, fullFileName)

					}
				}
			}
		}
	}

	odn, err := filepath.Abs(outputFolder) //Полный путь выходной папки
	if err != nil {
		slog.Default().ErrorContext(ctx, err.Error())
		return err
	}

	var wg sync.WaitGroup
	wg.Add(len(inputWordFiles))

	var media sync.Map

	//Проверяем, нет ли пути к файлам в значениях данных
	for k, v := range data {
		_ = k
		_, err := os.Stat(v)
		if err != nil {
			continue
		}
		if strings.ToLower(filepath.Ext(v)) == ".png" ||
			strings.ToLower(filepath.Ext(v)) == ".jpg" {
			media.Store(k, v)
		}
	}

	//Заполняем шаблоны
	work := func(fn string, data map[string]string) {
		// Создаём новый экземпляр FuncMap для каждой горутины.
		tmaps := makeTfm()

		defer wg.Done()
		if strings.ToLower(filepath.Ext(fn)) != ".docx" {
			_, err := filecopy(fn, filepath.Join(odn, strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))+strings.ToLower(filepath.Ext(fn))))
			if err != nil {
				slog.Default().ErrorContext(ctx, fmt.Errorf("template error 1 %w in file %s", err, fn).Error())
			}
			return
		}

		slog.Default().DebugContext(ctx, "Конвертируем файл", slog.String("file", filepath.Base(fn)))

		//новый шаблон
		jtpl, err := gotemplatedocx.NewDocxTemplateFromFilename(fn, gotemplatedocx.NoRemoveEmptyTableRows(), gotemplatedocx.RemoveRangeRows(), gotemplatedocx.IgnoreMissingKey())
		if err != nil {
			slog.Default().ErrorContext(ctx, fmt.Errorf("template error 2 %w in file %s", err, fn).Error())
			return
		}
		//доп функции
		jtpl.AddTemplateFuncs(tmaps)

		//грузим медию
		media.Range(func(key, value interface{}) bool {
			imageContent, err := os.ReadFile(value.(string))
			if err != nil {
				slog.Default().ErrorContext(ctx, fmt.Errorf("template error %w in file %s unable to read media %s", err, fn, value.(string)).Error())
				return true
			}
			jtpl.Media(value.(string), imageContent)
			return true // continue iteration
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
