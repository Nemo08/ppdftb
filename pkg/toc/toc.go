// Package toc реализует генерацию файла содержания (оглавления) в формате
// DOCX на основе шаблона и набора PDF-файлов с правильной нумерацией страниц.
package toc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"log/slog"

	"github.com/briiC/docxplate"
	pdf "github.com/oliverpool/unipdf/v3/model"
	"github.com/maruel/natural"
)

// OnePDFFile описывает один PDF-документ для оглавления.
type OnePDFFile struct {
	fileName  string
	cleanName string
	fullPath  string
	pages     uint
}

// TemplateData — данные для подстановки в DOCX-шаблон оглавления.
type TemplateData struct {
	Pages  []*TableData
	Number int
}

// TableData — строка оглавления: обозначение, наименование, страница.
type TableData struct {
	Obozn string
	Name  string
	Page  int
}

// Make генерирует файл оглавления DOCX на основе шаблона и PDF-файлов.
// templateFileName — путь к DOCX-шаблону оглавления.
// pdfDirectoryName — папка с PDF-файлами, для которых строится оглавление.
// compiledTemplateDirectoryName — папка для сохранения скомпилированного оглавления.
// templatePageNumber — номер страницы, с которой начинается оглавление.
func Make(ctx context.Context, templateFileName, pdfDirectoryName, compiledTemplateDirectoryName string, templatePageNumber int) error {
	tpn := templatePageNumber

	//Проверка наличия папок и шаблона
	slog.Default().DebugContext(ctx, "Проверка наличия папок и шаблона")
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
	slog.Default().DebugContext(ctx, "Читаем все файлы из pdf папки")
	allFiles, err := os.ReadDir(pdn)
	if err != nil {
		return err
	}

	//Получаем и сортируем список pdf файлов
	var PDFList []string
	for _, file := range allFiles {
		if !file.IsDir() {
			slog.Default().DebugContext(ctx, file.Name())
			if strings.ToLower(filepath.Ext(file.Name())) == ".pdf" {
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

	var pdfNumberedFileList []OnePDFFile
	var totalPages = 0
	var addOn = false

	for _, file := range PDFList {
		base := strings.TrimSuffix(file, filepath.Ext(file))
		var cn string
		if idx := strings.Index(base, " "); idx >= 0 {
			cn = strings.TrimSpace(base[idx:])
		} else {
			cn = base // нет пробела — берём всё имя
		}

		//Количество страниц в файле — читаем в замыкании, чтобы defer закрыл файл сразу.
		colPages := func() int {
			data, err := os.Open(filepath.Join(pdn, file))
			if err != nil {
				// Файл не существует — вероятно это ещё не созданный шаблон, считаем 1 страницу.
				return 1
			}
			defer data.Close()

			pdfReader, err := pdf.NewPdfReader(data)
			if err != nil {
				return 1
			}
			n, err := pdfReader.GetNumPages()
			if err != nil {
				return 1
			}
			return n
		}()

		if strings.TrimSuffix(file, filepath.Ext(file)) == strings.TrimSuffix(filepath.Base(tfn), filepath.Ext(tfn)) {
			addOn = true
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
	err = tdoc.ExportDocx(filepath.Join(ctdn, filepath.Base(tfn)))

	if err != nil {
		return err
	}

	return nil
}
