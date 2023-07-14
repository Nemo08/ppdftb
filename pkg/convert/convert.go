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

func FilesToPdf(ctx context.Context, sourceFile, sourceFolder, outputFolder string) error {
	var inputWordFiles []string

	//Если задан входной файл
	if sourceFile != "" {
		//Проверка наличия исходного файла
		if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
			slog.ErrorCtx(ctx, "Input file "+sourceFile+" does not exists")
		}

		ifn, err := filepath.Abs(sourceFile) //Полный путь входного файла
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
			return err
		}
		inputWordFiles = append(inputWordFiles, ifn)
	}

	//Если задана папка
	if sourceFolder != "" {
		//Проверка наличия исходной папки
		if _, err := os.Stat(sourceFolder); os.IsNotExist(err) {
			slog.ErrorCtx(ctx, "Directory "+sourceFolder+" does not exists")
		}

		//Читаем все файлы из исходной папки
		files, err := ioutil.ReadDir(sourceFolder)
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
			return err
		}

		//Ищем в папке doc, docx файлы
		for _, file := range files {
			if !file.IsDir() {
				if (strings.ToLower(path.Ext(file.Name())) == ".doc") || (strings.ToLower(path.Ext(file.Name())) == ".docx") {
					ffn, err := filepath.Abs(filepath.Join(sourceFolder, file.Name())) //Полный путь входного файла
					if err != nil {
						slog.ErrorCtx(ctx, err.Error())
						return err
					}
					inputWordFiles = append(inputWordFiles, ffn)
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
		slog.InfoCtx(ctx, "Конвертируем файл", slog.String("file", filepath.Base(fn)))

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
