package convert

import (
	"context"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/exp/slog"
)

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
							allFiles = append(allFiles, path.Join(stat.Name(), file.Name()))
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
