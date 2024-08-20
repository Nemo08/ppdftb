package pdf

import (
	"context"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/exp/slog"

	//unicommon "github.com/loxiouve/unipdf/v3/common"
	pdf "github.com/loxiouve/unipdf/v3/model"
	"github.com/maruel/natural"
)

func Merge(ctx context.Context, sourceFolder, outputFile string) error {
	var fileList []string

	//Читаем все файлы из исходной папки
	files, err := ioutil.ReadDir(sourceFolder)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}
	//Ищем в папке pdf файлы
	//TODO: проверка файлов на чтение
	//TODO: проверка выходного файла на блокировку
	for _, file := range files {
		if !file.IsDir() {
			if strings.ToLower(path.Ext(file.Name())) == ".pdf" {
				fileList = append(fileList, filepath.Join(sourceFolder, file.Name()))
			}
		}
	}

	//Не нашли pdf файлы в папке
	if len(fileList) == 0 {
		slog.InfoCtx(ctx, "В папке нет pdf файлов для объединения", slog.String("folder", sourceFolder))
		return nil
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
		colPages := 0
		data, err := os.Open(file)
		defer data.Close()

		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
			return err
		}

		//Создаем читалку pdf
		pdfReader, err := pdf.NewPdfReader(data)
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
			return err
		}

		//Получаем количество страниц в файле
		colPages, err = pdfReader.GetNumPages()
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
			return err
		}

		var currentPage *pdf.PdfPage
		var pcx, pcy float64

		//Проходим по страницам
		for p := 0; p < colPages; p++ {
			currentPage, err = pdfReader.GetPage(p + 1)
			if err != nil {
				slog.ErrorCtx(ctx, err.Error())
				return err
			}

			if p == 0 {
				pcx = currentPage.MediaBox.Height() * 0.98
				pcy = currentPage.MediaBox.Width() * 0.01
			}

			//Добавляем страницу в компановщик
			if err = pw.AddPage(currentPage); err != nil {
				slog.ErrorCtx(ctx, err.Error())
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
	}

	//Добавляем дерево закладок к собранному файлу
	pw.AddOutlineTree(otree.ToOutlineTree())

	//Создаем файл и пишем в него из буффера
	fo, err := os.Create(outputFile)
	slog.Debug("Вывод файла", slog.String("file", outputFile))
	defer fo.Close()
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}
	//unicommon.SetLogger(unicommon.NewConsoleLogger(unicommon.LogLevelTrace))
	//Пишем скомпанованный файл в буффер
	err = pw.Write(fo)

	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	return nil
}
