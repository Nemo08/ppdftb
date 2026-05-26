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
	"sync"

	"log/slog"

	"github.com/Nemo08/ppdftb/pkg/jobutil"
	"github.com/Nemo08/ppdftb/pkg/pdf"
	"github.com/briiC/docxplate"
	"github.com/maruel/natural"
)

// Option настраивает поведение Make.
type Option func(*options)

type options struct {
	pageCounts map[string]int // fileName → страницы (путь = filepath.Base)
}

// WithPageCounts передаёт заранее известное количество страниц для PDF-файлов.
// Ключ — filepath.Base(absPath), значение — количество страниц.
func WithPageCounts(counts map[string]int) Option {
	return func(o *options) {
		o.pageCounts = counts
	}
}

// OnePDFFile описывает один PDF-документ для оглавления.
type OnePDFFile struct {
	fileName  string
	obozn     string
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

// splitFileBase разбивает имя файла на обозначение (до первого пробела) и название (после).
// Пример: "01-01-ABC Проект здания" → ("01-01-ABC", "Проект здания").
func splitFileBase(base string) (obozn, name string) {
	if idx := strings.Index(base, " "); idx >= 0 {
		return strings.TrimSpace(base[:idx]), strings.TrimSpace(base[idx:])
	}
	return base, base
}

// Make генерирует файл оглавления DOCX на основе шаблона и PDF-файлов.
// templateFileName — путь к DOCX-шаблону оглавления.
// pdfDirectoryName — папка с PDF-файлами, для которых строится оглавление.
// compiledTemplateDirectoryName — папка для сохранения скомпилированного оглавления.
// templatePageNumber — номер страницы, с которой начинается оглавление.
// opts — опции: WithPageCounts — заранее известное количество страниц.
func Make(ctx context.Context, templateFileName, pdfDirectoryName, compiledTemplateDirectoryName string, templatePageNumber int, opts ...Option) error {
	var o options
	for _, fn := range opts {
		fn(&o)
	}

	pdn, ctdn, tfn, err := resolveTocPaths(pdfDirectoryName, compiledTemplateDirectoryName, templateFileName)
	if err != nil {
		return err
	}

	PDFList, err := collectPdfFiles(ctx, pdn, tfn)
	if err != nil {
		return err
	}

	pdfNumberedFileList := buildPdfFileList(PDFList, pdn, tfn, o.pageCounts)

	td := buildTemplateData(pdfNumberedFileList, templatePageNumber)

	tdoc, err := docxplate.OpenTemplate(tfn)
	if err != nil {
		return err
	}

	tdoc.Params(td)
	return tdoc.ExportDocx(filepath.Join(ctdn, filepath.Base(tfn)))
}

func resolveTocPaths(pdfDir, compiledDir, templateFile string) (pdn, ctdn, tfn string, err error) {
	if _, err := os.Stat(pdfDir); os.IsNotExist(err) {
		return "", "", "", errors.New("Папка " + pdfDir + " не существует")
	}
	pdn, err = filepath.Abs(pdfDir)
	if err != nil {
		return "", "", "", err
	}

	if _, err := os.Stat(compiledDir); os.IsNotExist(err) {
		return "", "", "", errors.New("Папка " + compiledDir + " не существует")
	}
	ctdn, err = filepath.Abs(compiledDir)
	if err != nil {
		return "", "", "", err
	}

	if _, err := os.Stat(templateFile); os.IsNotExist(err) {
		return "", "", "", errors.New("Файл " + templateFile + " не существует")
	}
	tfn, err = filepath.Abs(templateFile)
	if err != nil {
		return "", "", "", err
	}
	return pdn, ctdn, tfn, nil
}

func collectPdfFiles(ctx context.Context, pdn, tfn string) ([]string, error) {
	slog.Default().DebugContext(ctx, "Читаем все файлы из pdf папки")
	PDFList, err := pdf.CollectPdfFiles(pdn)
	if err != nil {
		return nil, err
	}

	// Извлекаем только имена (CollectPdfFiles возвращает полные пути)
	var names []string
	for _, f := range PDFList {
		names = append(names, filepath.Base(f))
	}

	templatePdfName := strings.TrimSuffix(filepath.Base(tfn), filepath.Ext(tfn)) + ".pdf"
	templateFoundInPdf := false
	for _, file := range names {
		if file == templatePdfName {
			templateFoundInPdf = true
			break
		}
	}
	if !templateFoundInPdf {
		names = append(names, templatePdfName)
	}

	if len(names) == 0 {
		return nil, errors.New("Папка " + pdn + " не содержит pdf файлов")
	}

	sort.Sort(natural.StringSlice(names))
	return names, nil
}

func extractCleanName(base string) string {
	if idx := strings.Index(base, " "); idx >= 0 {
		return strings.TrimSpace(base[idx:])
	}
	return base
}

func getPdfPageCount(filePath string) int {
	n, err := pdf.PageCount(filePath)
	if err != nil {
		slog.Default().Warn("getPdfPageCount", slog.String("file", filePath), slog.String("err", err.Error()))
		return 1
	}
	return n
}

func buildPdfFileList(PDFList []string, pdn, tfn string, pageCounts map[string]int) []OnePDFFile {
	if pageCounts == nil {
		pageCounts = make(map[string]int)
	}
	// Параллельное получение количества страниц для файлов, не попавших в кэш
	var needFetch []string
	for _, file := range PDFList {
		if _, ok := pageCounts[file]; !ok {
			needFetch = append(needFetch, file)
		}
	}
	if len(needFetch) > 0 {
		var mu sync.Mutex
		jobutil.Parallel(8, needFetch, func(file string) {
			pages := getPdfPageCount(filepath.Join(pdn, file))
			mu.Lock()
			pageCounts[file] = pages
			mu.Unlock()
		})
	}

	var result []OnePDFFile
	templateBase := strings.TrimSuffix(filepath.Base(tfn), filepath.Ext(tfn))

	// Согласно ГОСТ Р 2.105-2019, регламентирующему правила оформления технической
	// документации в РФ, титульный лист не является разделом текста и не включается
	// в содержание. Само «Содержание» также не вносится в свой собственный перечень.
	// Поэтому из списка исключаются все файлы идущие до шаблона содержания включительно:
	// PDFList отсортирован natural-sort, шаблон содержания определяется по templateBase.
	pastTemplate := false
	for _, file := range PDFList {
		base := strings.TrimSuffix(file, filepath.Ext(file))
		if base == templateBase {
			pastTemplate = true
			continue
		}
		if !pastTemplate {
			continue
		}
		obozn, cn := splitFileBase(base)

		colPages, _ := pageCounts[file]

		p, err := filepath.Abs(filepath.Join(pdn, file))
		if err != nil {
			continue
		}
		result = append(result, OnePDFFile{
			fileName:  file,
			obozn:     obozn,
			fullPath:  p,
			cleanName: cn,
			pages:     uint(colPages),
		})
	}
	return result
}

func buildTemplateData(files []OnePDFFile, startPage int) TemplateData {
	td := TemplateData{}
	currPageNumber := startPage
	for _, v := range files {
		td.Pages = append(td.Pages, &TableData{Obozn: v.obozn, Name: v.cleanName, Page: currPageNumber})
		currPageNumber += int(v.pages)
	}
	return td
}
