package convert

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"golang.org/x/exp/slog"

	"github.com/Nemo08/ppdftb/pkg/agte"
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

func TplToDocx(ctx context.Context, source []string, outputFolder string, data map[string]string) error {
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
					if strings.ToLower(filepath.Ext(v)) == ".doc" || strings.ToLower(filepath.Ext(v)) == ".docx" || strings.ToLower(filepath.Ext(v)) == ".rtf" {
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

	work := func(fn string, data map[string]string) {
		defer wg.Done()
		if strings.ToLower(filepath.Ext(fn)) != ".docx" {
			_, err := filecopy(fn, filepath.Join(odn, strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))+strings.ToLower(filepath.Ext(fn))))
			if err != nil {
				slog.Default().ErrorContext(ctx, fmt.Errorf("template error 1 %w in file %s", err, fn).Error())
			}
			return
		}

		slog.Default().DebugContext(ctx, "Конвертируем файл", slog.String("file", filepath.Base(fn)))

		tpl := agte.NewTemplate(ctx, AdditionalFuncs)
		err := tpl.Open(fn)
		if err != nil {
			slog.Default().ErrorContext(ctx, fmt.Errorf("template error 2 %w in file %s", err, fn).Error())
			return
		}

		err = tpl.Render(data)
		if err != nil {
			slog.Default().ErrorContext(ctx, fmt.Errorf("template error 3 %w in file %s", err, fn).Error())
			return
		}

		err = tpl.SaveTo(filepath.Join(odn, strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))+".docx"))
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

func GetData(ctx context.Context, source []string) (map[string]string, error) {
	var tempMap Map

	for _, path := range source {
		content, err := os.ReadFile(path)
		if err != nil {
			return tempMap, err
		}

		xml.Unmarshal(content, (*Map)(&tempMap))
	}
	return tempMap, nil
}

func (m *Map) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type xmlMapEntry struct {
		XMLName xml.Name
		Value   string `xml:",chardata"`
	}

	*m = Map{}
	for {
		var e xmlMapEntry

		err := d.Decode(&e)
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		(*m)[(e.XMLName.Local)] = e.Value
	}
	return nil
}

// makeTfm создаёт новый экземпляр FuncMap для каждого вызова.
// Нельзя использовать одну глобальную map из нескольких горутин —
// gotemplatedocx и text/template пишут в неё при вызове Funcs/Apply,
// что вызывает гонку "concurrent map iteration and map write".
func makeTfm() template.FuncMap {
	return template.FuncMap{
		"year": func() (string, error) {
			return strconv.Itoa(time.Now().Year()), nil
		},
		"datetime": func() (string, error) {
			return time.Now().Format("02.01.2006 15:04"), nil
		},
		"nowdate": func() (string, error) {
			return time.Now().Format("02.01.2006") + " ", nil
		},
		"datetimeof": func(path string) (string, error) {
			fileinfo, err := os.Stat(path)
			if err != nil {
				return time.Now().Format("02.01.2006 15:04"), err
			}
			atime := fileinfo.ModTime()
			return atime.Format("15:04 02.01.2006"), nil
		},
		"sizeof": func(path string) (string, error) {
			fileinfo, err := os.Stat(path)
			if err != nil {
				return "", err
			}
			size := fileinfo.Size()
			return strconv.FormatInt(size, 10), nil
		},
		"crc32of": func(path string) (string, error) {
			dat, err := os.ReadFile(path)
			if err != nil {
				return time.Now().Format("02.01.2006 15:04"), err
			}
			const p = 0b11101101101110001000001100100000
			cksum := crc32.MakeTable(p)
			return strconv.FormatInt(int64(crc32.Checksum(dat, cksum)), 16), nil
		},
	}
}

func AdditionalFuncs(t *template.Template) {
	// Каждый вызов создаёт новую map — безопасно из нескольких горутин.
	t.Funcs(makeTfm())
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
