package toc

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/exp/slog"

	"github.com/briiC/docxplate"
	pdf "github.com/loxiouve/unipdf/v3/model"
	"github.com/maruel/natural"
)

type OnePDFFile struct {
	fileName   string
	cleanName  string
	fullPath   string
	pages      uint
	pageNumber uint
}

type TemplateData struct {
	Pages  []*TableData
	Number int
}
type TableData struct {
	Obozn string
	Name  string
	Page  int
}

var pdfNumberedFileList []OnePDFFile

func Make(ctx context.Context, templateFileName, pdfDirectoryName, compiledTemplateDirectoryName string, templatePageNumber int) error {
	tpn := templatePageNumber

	//Проверка наличия папок и шаблона
	slog.DebugCtx(ctx, "Проверка наличия папок и шаблона")
	if _, err := os.Stat(pdfDirectoryName); os.IsNotExist(err) {
		return errors.New("Папка " + pdfDirectoryName + " не существует")
	}

	pdn, err := filepath.Abs(pdfDirectoryName)
	if err != nil {
		return err
	}

	if _, err := os.Stat(compiledTemplateDirectoryName); os.IsNotExist(err) {
		return errors.New("Папка " + compiledTemplateDirectoryName + " не существует")
	}
	ctdn, err := filepath.Abs(compiledTemplateDirectoryName)

	if err != nil {
		return err
	}

	if _, err := os.Stat(templateFileName); os.IsNotExist(err) {
		return errors.New("Файл " + templateFileName + " не существует")
	}

	tfn, err := filepath.Abs(templateFileName)
	if err != nil {
		return err
	}

	//Читаем все файлы из pdf папки
	slog.DebugCtx(ctx, "Читаем все файлы из pdf папки")
	allFiles, err := ioutil.ReadDir(pdn)
	if err != nil {
		return err
	}

	//Получаем и сортируем список pdf файлов
	var PDFList []string
	for _, file := range allFiles {
		if !file.IsDir() {
			slog.DebugCtx(ctx, file.Name())
			if strings.ToLower(path.Ext(file.Name())) == ".pdf" {
				PDFList = append(PDFList, file.Name())
			}
			if file.Size() == 0 {
				PDFList = append(PDFList, file.Name())
			}
		}
	}

	templateFoundInPdf := false
	templatePdfName := strings.TrimSuffix(filepath.Base(tfn), filepath.Ext(tfn)) + ".pdf"

	//Проверяем, есть ли в папке уже собранный шаблон
	for _, file := range PDFList {
		if file == templatePdfName {
			templateFoundInPdf = true
		}
	}
	if templateFoundInPdf == false {
		//Добавляем если нет
		PDFList = append(PDFList, templatePdfName)
	}

	//Не нашли pdf файлы в папке
	if len(PDFList) == 0 {
		return errors.New("Папка " + pdn + " не содержит pdf файлов")
	}

	//Сортируем слайс "естественной" сортировкой
	sort.Sort(natural.StringSlice(PDFList))

	var totalPages = 0
	var addOn = false

	for _, file := range PDFList {
		cn := strings.TrimSuffix(file, filepath.Ext(file))
		cn = strings.TrimSpace(cn[strings.Index(strings.TrimSuffix(file, filepath.Ext(file)), " "):])

		var colPages int //Количество страниц в файле
		data, err := os.Open(filepath.Join(pdn, file))

		if err != nil { //Если файл не существующий, но уже есть в нашем списке, то это вероятно шаблон
			colPages = 1
		} else { //Нормальный pdf файл
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
			data.Close()
		}

		if strings.TrimSuffix(file, filepath.Ext(file)) == strings.TrimSuffix(filepath.Base(tfn), filepath.Ext(tfn)) {
			addOn = true
			//tpn = totalPages + 1
		}

		if addOn {
			p, err := filepath.Abs(filepath.Join(pdn, file))
			if err != nil {
				return err
			}

			pdfNumberedFileList = append(
				pdfNumberedFileList,
				OnePDFFile{
					fileName:  file,
					fullPath:  p,
					cleanName: cn,
					pages:     uint(colPages),
				})
		}
		totalPages += colPages
	}

	//Первое формирование TOC без количества листов содержания
	td := TemplateData{}
	currPageNumber := tpn
	for _, v := range pdfNumberedFileList {
		td.Pages = append(td.Pages, &TableData{Name: v.cleanName, Page: currPageNumber})
		currPageNumber += int(v.pages)
	}

	tdoc, err := docxplate.OpenTemplate(tfn)
	if err != nil {
		return err
	}

	tdoc.Params(td)
	err = tdoc.ExportDocx(path.Join(ctdn, filepath.Base(tfn)))

	if err != nil {
		return err
	}

	return nil
}
