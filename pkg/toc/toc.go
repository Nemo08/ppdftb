// Package toc реализует генерацию файла содержания (оглавления) в формате
// DOCX на основе шаблона и набора PDF-файлов с правильной нумерацией страниц.
package toc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"log/slog"

	gotemplatedocx "github.com/JJJJJJack/go-template-docx"
	"github.com/Nemo08/ppdftb/pkg/jobutil"
	"github.com/Nemo08/ppdftb/pkg/pdf"
	"github.com/maruel/natural"
)

// Option настраивает поведение Make.
type Option func(*options)

type options struct {
	pageCounts   map[string]int // fileName → страницы (путь = filepath.Base)
	appendix     bool           // режим приложений
	templateData []byte         // общие данные штампа (merged XML) для подстановки
}

// WithAppendix включает режим приложений (автодетект маркеров-разделителей).
func WithAppendix() Option {
	return func(o *options) {
		o.appendix = true
	}
}

// WithPageCounts передаёт заранее известное количество страниц для PDF-файлов.
// Ключ — filepath.Base(absPath), значение — количество страниц.
func WithPageCounts(counts map[string]int) Option {
	return func(o *options) {
		o.pageCounts = counts
	}
}

// WithTemplateData передаёт общие данные штампа (merged XML) для подстановки
// в шаблон содержания помимо TemplateData (Pages/Number). Плейсхолдеры
// общих полей (ESNumber, ESType и т.п.) заполняются сразу при генерации.
func WithTemplateData(data []byte) Option {
	return func(o *options) {
		o.templateData = data
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
	Pages  []*TableData `json:"pages"`
	Number int          `json:"number"`
}

// TableData — строка оглавления: обозначение, наименование, страница.
type TableData struct {
	Obozn string `json:"obozn"`
	Name  string `json:"name"`
	Page  string `json:"page"`
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

	if o.appendix {
		td, err := makeAppendixToc(ctx, pdn, tfn, templatePageNumber, o.pageCounts)
		if err != nil {
			return err
		}
		return exportTocDocx(tfn, ctdn, td, o.templateData)
	}

	PDFList, err := collectPdfFiles(ctx, pdn, tfn)
	if err != nil {
		return err
	}

	pdfNumberedFileList := buildPdfFileList(PDFList, pdn, tfn, o.pageCounts)

	td := buildTemplateData(pdfNumberedFileList, templatePageNumber)
	return exportTocDocx(tfn, ctdn, &td, o.templateData)
}

// makeAppendixToc строит оглавление в режиме приложений.
// Учитывает заглушки-разделители (KindDivider) и приложения (KindAppendix).
func makeAppendixToc(ctx context.Context, pdn, tfn string, startPage int, pageCounts map[string]int) (*TemplateData, error) {
	slog.Default().DebugContext(ctx, "makeAppendixToc: collecting entries with appendix mode")

	entries, err := pdf.CollectEntries(pdn, true)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, errors.New("Папка " + pdn + " не содержит файлов")
	}

	templateBase := strings.TrimSuffix(filepath.Base(tfn), filepath.Ext(tfn))
	slog.Default().DebugContext(ctx, "makeAppendixToc",
		slog.String("templateBase", templateBase),
		slog.Int("entries", len(entries)))

	entries, templateIdx := insertTemplateEntry(entries, templateBase)

	// Отбираем entry после шаблона содержания.
	after := entriesAfterTemplate(entries, templateIdx)
	if len(after) == 0 {
		slog.Default().DebugContext(ctx, "no entries after template")
		return &TemplateData{}, nil
	}

	pageCounts = fetchMissingPageCounts(after, pageCounts)

	return buildAppendixTableData(after, startPage, pageCounts), nil
}

// insertTemplateEntry находит позицию шаблона содержания в entries.
// Сравнивает с RawName (имя без расширения) каждого entry.
// Если шаблон не найден (PDF ещё нет), создаёт виртуальный entry
// и вставляет на правильную позицию (как делает collectPdfFiles
// для обычного режима). Возвращает обновлённый список и индекс шаблона.
func insertTemplateEntry(entries []pdf.FileEntry, templateBase string) ([]pdf.FileEntry, int) {
	for i, e := range entries {
		if e.RawName == templateBase {
			return entries, i
		}
	}

	idx := sort.Search(len(entries), func(i int) bool {
		return natural.Less(templateBase, entries[i].RawName)
	})
	synthetic := pdf.FileEntry{
		FullPath: "",
		Name:     "",
		RawName:  templateBase,
		Kind:     pdf.KindNormal,
	}
	entries = append(entries, pdf.FileEntry{})
	copy(entries[idx+1:], entries[idx:])
	entries[idx] = synthetic
	return entries, idx
}

// entriesAfterTemplate возвращает entry после шаблона содержания включительно.
func entriesAfterTemplate(entries []pdf.FileEntry, templateIdx int) []pdf.FileEntry {
	var after []pdf.FileEntry
	for i, e := range entries {
		if i <= templateIdx {
			continue // пропускаем до шаблона включительно
		}
		after = append(after, e)
	}
	return after
}

// fetchMissingPageCounts параллельно получает количество страниц для всех
// PDF-файлов после шаблона, отсутствующих в pageCounts. У разделителей
// (KindDivider) страниц нет — они пропускаются.
// Если pageCounts nil — создаётся новая карта. Возвращает заполненную карту.
func fetchMissingPageCounts(after []pdf.FileEntry, pageCounts map[string]int) map[string]int {
	if pageCounts == nil {
		pageCounts = make(map[string]int)
	}
	var needFetch []string
	for _, e := range after {
		if e.Kind == pdf.KindDivider {
			continue
		}
		baseName := filepath.Base(e.FullPath)
		if _, ok := pageCounts[baseName]; !ok {
			needFetch = append(needFetch, e.FullPath)
		}
	}
	if len(needFetch) > 0 {
		var mu sync.Mutex
		jobutil.Parallel(8, needFetch, func(fullPath string) {
			pages := getPdfPageCount(fullPath)
			mu.Lock()
			pageCounts[filepath.Base(fullPath)] = pages
			mu.Unlock()
		})
	}
	return pageCounts
}

// buildAppendixTableData строит TemplateData из entries после шаблона.
func buildAppendixTableData(after []pdf.FileEntry, startPage int, pageCounts map[string]int) *TemplateData {
	td := &TemplateData{}
	currPage := startPage
	for _, e := range after {
		switch e.Kind {
		case pdf.KindDivider:
			td.Pages = append(td.Pages, &TableData{
				Obozn: "",
				Name:  e.BookTitle,
				Page:  "",
			})
		case pdf.KindAppendix:
			td.Pages = append(td.Pages, &TableData{
				Obozn: "",
				Name:  "Приложение " + e.Letter + ". " + e.Name,
				Page:  strconv.Itoa(currPage),
			})
			baseName := filepath.Base(e.FullPath)
			currPage += pageCounts[baseName]
		case pdf.KindNormal:
			td.Pages = append(td.Pages, &TableData{
				Obozn: "",
				Name:  e.Name,
				Page:  strconv.Itoa(currPage),
			})
			baseName := filepath.Base(e.FullPath)
			currPage += pageCounts[baseName]
		}
	}
	return td
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

// exportTocDocx сохраняет TemplateData в DOCX через go-template-docx.
// templateData — общие данные штампа (merged XML); если непустые,
// они мёржатся с TemplateData (Pages/Number приоритетны) для подстановки
// в шаблон содержания помимо строк оглавления.
func exportTocDocx(tfn, ctdn string, td *TemplateData, templateData []byte) error {
	//nolint:gosec // G304: tfn — путь к шаблону из CLI
	docxBytes, err := os.ReadFile(tfn)
	if err != nil {
		return err
	}

	data, err := mergeTemplateData(td, templateData)
	if err != nil {
		return err
	}

	result, err := gotemplatedocx.Render(docxBytes, data,
		gotemplatedocx.WithIgnoreMissingKey(true),
		gotemplatedocx.WithAutoExpandRows(data),
		gotemplatedocx.WithRemoveEmptyTableRows(false),
	)
	if err != nil {
		return err
	}

	outPath := filepath.Join(ctdn, filepath.Base(tfn))
	//nolint:gosec // G703: outPath строится из фиксированного каталога и имени шаблона
	return os.WriteFile(outPath, result, 0600)
}

// mergeTemplateData объединяет TemplateData (Pages/Number) с общими данными
// штампа. Ключи TemplateData имеют приоритет над общими данными.
// Если общие данные пусты — возвращается JSON только TemplateData.
func mergeTemplateData(td *TemplateData, templateData []byte) ([]byte, error) {
	tdJSON, err := json.Marshal(td)
	if err != nil {
		return nil, err
	}
	if len(templateData) == 0 {
		return tdJSON, nil
	}

	merged := make(map[string]any)
	if err := json.Unmarshal(templateData, &merged); err != nil {
		return nil, fmt.Errorf("разбор общих данных штампа: %w", err)
	}
	var tdMap map[string]any
	if err := json.Unmarshal(tdJSON, &tdMap); err != nil {
		return nil, err
	}
	maps.Copy(merged, tdMap)
	return json.Marshal(merged)
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
	templateFoundInPdf := slices.Contains(names, templatePdfName)
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

func buildPdfFileList(pdfList []string, pdn, tfn string, pageCounts map[string]int) []OnePDFFile {
	if pageCounts == nil {
		pageCounts = make(map[string]int)
	}
	// Параллельное получение количества страниц для файлов, не попавших в кэш
	var needFetch []string
	for _, file := range pdfList {
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
	// pdfList отсортирован natural-sort, шаблон содержания определяется по templateBase.
	pastTemplate := false
	for _, file := range pdfList {
		base := strings.TrimSuffix(file, filepath.Ext(file))
		if base == templateBase {
			pastTemplate = true
			continue
		}
		if !pastTemplate {
			continue
		}
		obozn, cn := splitFileBase(base)

		colPages := pageCounts[file]

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
		td.Pages = append(td.Pages, &TableData{Obozn: v.obozn, Name: v.cleanName, Page: strconv.Itoa(currPageNumber)})
		currPageNumber += int(v.pages)
	}
	return td
}
