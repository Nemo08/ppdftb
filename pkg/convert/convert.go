package convert

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/ioutil"
	"os"
	"path"
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

func FilesToPdf(ctx context.Context, source []string, outputFolder string) error {
	var inputWordFiles []string

	for _, v := range source {
		//Проверка наличия
		if stat, err := os.Stat(v); os.IsNotExist(err) {
			slog.ErrorCtx(ctx, v+" не существует")
		} else {
			if !strings.HasPrefix(stat.Name(), "~$") {
				var allFiles []string

				//Собираем список всех файлов и файлов в папках
				if stat.IsDir() {
					files, err := ioutil.ReadDir(v)
					if err != nil {
						slog.ErrorCtx(ctx, err.Error())
						return err
					}
					for _, file := range files {
						if !file.IsDir() {
							allFiles = append(allFiles, path.Join(v, file.Name()))
						}
					}
				} else {
					allFiles = append(allFiles, v)
				}

				//Фильтруем список файлов
				for _, v := range allFiles {
					if (strings.ToLower(path.Ext(v)) == ".doc") || (strings.ToLower(path.Ext(v)) == ".docx") || (strings.ToLower(path.Ext(v)) == ".rtf") {
						fullFileName, err := filepath.Abs(v) //Полный путь входного файла
						if err != nil {
							slog.ErrorCtx(ctx, err.Error())
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
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	var wg sync.WaitGroup
	wg.Add(len(inputWordFiles))

	work := func(fn string) {
		defer wg.Done()
		slog.DebugCtx(ctx, "Конвертируем файл", slog.String("file", filepath.Base(fn)))

		err := WordToPdf(ctx, fn, filepath.Join(odn, strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))+".pdf"))
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
		}
	}

	for _, f := range inputWordFiles {
		go work(f)
	}
	wg.Wait()
	return nil
}

func TplToDocx(ctx context.Context, source []string, outputFolder string, data map[string]string) error {
	var inputWordFiles []string
	var err error

	for _, v := range source {
		//Проверка наличия
		if stat, err := os.Stat(v); os.IsNotExist(err) {
			slog.ErrorCtx(ctx, v+" не существует")
		} else {
			if !strings.HasPrefix(stat.Name(), "~$") {
				var allFiles []string

				//Собираем список всех файлов и файлов в папках
				if stat.IsDir() {
					files, err := ioutil.ReadDir(v)
					if err != nil {
						slog.ErrorCtx(ctx, err.Error())
						return err
					}
					for _, file := range files {
						if !file.IsDir() {
							allFiles = append(allFiles, path.Join(v, file.Name()))
						}
					}
				} else {
					allFiles = append(allFiles, v)
				}

				//Фильтруем список файлов
				for _, v := range allFiles {
					if (strings.ToLower(path.Ext(v)) == ".doc") || (strings.ToLower(path.Ext(v)) == ".docx") || (strings.ToLower(path.Ext(v)) == ".rtf") {
						fullFileName, err := filepath.Abs(v) //Полный путь входного файла
						if err != nil {
							slog.ErrorCtx(ctx, err.Error())
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
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	var wg sync.WaitGroup
	wg.Add(len(inputWordFiles))

	work := func(fn string, data map[string]string) {
		defer wg.Done()
		if strings.ToLower(path.Ext(fn)) != ".docx" {
			_, err := filecopy(fn, filepath.Join(odn, strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))+strings.ToLower(path.Ext(fn))))
			if err != nil {
				slog.ErrorCtx(ctx, err.Error())
			}
			return
		}

		slog.DebugCtx(ctx, "Конвертируем файл", slog.String("file", filepath.Base(fn)))

		tpl := agte.NewTemplate(ctx, AdditionalFuncs)
		err := tpl.Open(fn)
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
		}

		err = tpl.Render(data)
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
		}

		err = tpl.SaveTo(filepath.Join(odn, strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))+".docx"))
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
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
		content, err := ioutil.ReadFile(path)
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

func AdditionalFuncs(t *template.Template) {
	t.Funcs(
		template.FuncMap{
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

				fmt.Println()
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
		})
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
