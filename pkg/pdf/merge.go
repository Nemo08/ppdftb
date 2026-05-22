package pdf

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"log/slog"

	pdf "github.com/loxiouve/unipdf/v3/model"
	"github.com/maruel/natural"
)

// Merge объединяет все PDF-файлы из sourceFolder в один outputFile.
// Файлы сортируются natural sort; каждый файл становится закладкой верхнего уровня,
// внутренние закладки (outline) подшиваются под неё.
func Merge(ctx context.Context, sourceFolder, outputFile string) error {
	var fileList []string

	//Читаем все файлы из исходной папки
	files, err := os.ReadDir(sourceFolder)
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return err
	}
	//Ищем в папке pdf файлы
	for _, file := range files {
		if !file.IsDir() {
			if strings.ToLower(path.Ext(file.Name())) == ".pdf" {
				fileList = append(fileList, filepath.Join(sourceFolder, file.Name()))
			}
		}
	}

	//Не нашли pdf файлы в папке
	if len(fileList) == 0 {
		slog.InfoContext(ctx, "В папке нет pdf файлов для объединения", slog.String("folder", sourceFolder))
		return nil
	}

	//Проверяем, что все файлы читаются
	for _, f := range fileList {
		r, err := os.Open(f)
		if err != nil {
			return fmt.Errorf("файл %q не читается: %w", f, err)
		}
		r.Close()
	}

	//Сортируем слайс "естественной" сортировкой
	sort.Sort(natural.StringSlice(fileList))

	//Создаем дерево закладок
	otree := pdf.NewOutline()

	//Создаем компановщик pdf
	pw := pdf.NewPdfWriter()

	//ot := pdf.PdfOutlineTreeNode{}
	totalPages := 0

	//Проходим по списку pdf-ок
	for _, file := range fileList {
		err := func() error {
			colPages := 0
			data, err := os.Open(file)
			if err != nil {
				return err
			}
			defer data.Close()

			//Создаем читалку pdf
			pdfReader, err := pdf.NewPdfReader(data)
			if err != nil {
				return err
			}

			//Получаем количество страниц в файле
			colPages, err = pdfReader.GetNumPages()
			if err != nil {
				return err
			}

			var currentPage *pdf.PdfPage
			var pcx, pcy float64

			//Проходим по страницам
			for p := 0; p < colPages; p++ {
				currentPage, err = pdfReader.GetPage(p + 1)
				if err != nil {
					return err
				}

				if p == 0 {
					pcx = currentPage.MediaBox.Height() * 0.98
					pcy = currentPage.MediaBox.Width() * 0.01
				}

				//Добавляем страницу в компановщик
				if err = pw.AddPage(currentPage); err != nil {
					return err
				}
			}

			link := totalPages
			if link < 0 {
				link = 0
			}

			//Создаем закладку верхнего уровня с именем файла
			linkText := strings.TrimSuffix(filepath.Base(file), filepath.Ext(filepath.Base(file)))
			oi := pdf.NewOutlineItem(
				linkText,
				pdf.NewOutlineDest(int64(link), pcy, pcx))

			currOI, err := pdfReader.GetOutlines()
			if err == nil {
				//если в файле есть свои закладки добавляем их подзакладки
				for _, v := range currOI.Items() {
					oi.Add(v)
				}
			}

			//Добавляем закладки из файла к верхнему уровню
			otree.Add(oi)
			totalPages += colPages
			return nil
		}()
		if err != nil {
			slog.ErrorContext(ctx, err.Error())
			return err
		}
	}

	//Добавляем дерево закладок к собранному файлу
	pw.AddOutlineTree(otree.ToOutlineTree())

	//Пишем во временный файл, затем переименовываем —
	//атомарная операция, не оставляет битый файл при сбое.
	tmpFile := outputFile + ".tmp"
	fo, err := os.Create(tmpFile)
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return err
	}
	slog.Debug("Вывод файла", slog.String("file", tmpFile))
	err = pw.Write(fo)
	fo.Close()
	if err != nil {
		os.Remove(tmpFile)
		slog.ErrorContext(ctx, err.Error())
		return err
	}

	if err := os.Rename(tmpFile, outputFile); err != nil {
		os.Remove(tmpFile)
		slog.ErrorContext(ctx, err.Error())
		return err
	}

	return nil
}
