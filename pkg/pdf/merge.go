package pdf

import (
	"context"
	"fmt"
	"os"
	"strings"

	"log/slog"

	"github.com/Nemo08/ppdftb/pkg/fileutil"
	pdf "github.com/oliverpool/unipdf/v3/model"
)

// MergeOptions — настройки сборки PDF.
type MergeOptions struct {
	// Appendix включает распознавание файлов-заглушек и нумерацию приложений
	// буквами по ГОСТ Р 2.105-2019. Заглушки (файлы без расширения) становятся
	// закладками-разделителями в outline; файлы между маркерами ПРИЛОЖЕНИЯ и
	// ГРАФИЧЕСКАЯ ЧАСТЬ получают обозначение «Приложение А. Название».
	Appendix bool
}

// MergeOption — функциональная опция для Merge.
type MergeOption func(*MergeOptions)

// WithAppendix включает режим приложений.
func WithAppendix() MergeOption {
	return func(o *MergeOptions) { o.Appendix = true }
}

// Merge объединяет все PDF-файлы из sourceFolder в один outputFile.
// Файлы сортируются natural sort; каждый файл становится закладкой верхнего уровня.
func Merge(ctx context.Context, sourceFolder, outputFile string, opts ...MergeOption) error {
	o := &MergeOptions{}
	for _, opt := range opts {
		opt(o)
	}

	slog.Debug("Merge called",
		slog.String("sourceFolder", sourceFolder),
		slog.String("outputFile", outputFile),
		slog.Bool("appendix", o.Appendix))

	entries, err := CollectEntries(sourceFolder, o.Appendix)
	if err != nil {
		return err
	}
	slog.Debug("Merge entries collected", slog.Int("count", len(entries)))

	// Фильтруем — только PDF
	var pdfEntries []FileEntry
	for _, e := range entries {
		if e.Kind == KindDivider {
			slog.Debug("Merge skipping divider", slog.String("title", e.BookTitle))
			continue // заглушки не добавляем как страницы
		}
		pdfEntries = append(pdfEntries, e)
	}

	if len(pdfEntries) == 0 {
		slog.DebugContext(ctx, "В папке нет pdf файлов для объединения",
			slog.String("folder", sourceFolder))
		return nil
	}

	pw := pdf.NewPdfWriter()
	otree := pdf.NewOutline()

	// Добавляем заглушки-разделители в outline (без страниц).
	// Нужно строить outline параллельно с добавлением страниц.
	totalPages := 0

	for _, entry := range entries {
		if entry.Kind == KindDivider {
			// Закладка-разделитель: указываем на следующую PDF-страницу
			oi := pdf.NewOutlineItem(entry.BookTitle,
				pdf.NewOutlineDest(int64(totalPages), 0, 0))
			otree.Add(oi)
			continue
		}
		if err := mergeOneEntry(entry, &pw, otree, &totalPages); err != nil {
			slog.ErrorContext(ctx, err.Error())
			return err
		}
	}

	pw.AddOutlineTree(otree.ToOutlineTree())
	return writeOutput(&pw, outputFile)
}

func mergeOneEntry(entry FileEntry, pw *pdf.PdfWriter, otree *pdf.Outline, totalPages *int) error {
	slog.Debug("merging entry",
		slog.String("name", entry.Name),
		slog.String("booktitle", entry.BookTitle),
		slog.Int("kind", int(entry.Kind)))

	colPages, pcx, pcy, pdfReader, err := readAndAddPages(entry.FullPath, pw)
	if err != nil {
		return fmt.Errorf("файл %q: %w", entry.RawName, err)
	}

	title := entry.BookTitle
	if title == "" {
		title = entry.Name
	}

	oi := createOutlineItemWithTitle(title, float64(*totalPages), pcx, pcy, pdfReader)
	otree.Add(oi)
	*totalPages += colPages
	return nil
}

func readAndAddPages(file string, pw *pdf.PdfWriter) (int, float64, float64, *pdf.PdfReader, error) {
	data, err := os.Open(file)
	if err != nil {
		return 0, 0, 0, nil, err
	}
	defer func() {
		if err := data.Close(); err != nil {
			slog.Warn("readAndAddPages close", slog.String("file", file), slog.String("error", err.Error()))
		}
	}()

	pdfReader, err := pdf.NewPdfReader(data)
	if err != nil {
		return 0, 0, 0, nil, err
	}

	colPages, err := pdfReader.GetNumPages()
	if err != nil {
		return 0, 0, 0, nil, err
	}

	var pcx, pcy float64
	for p := range colPages {
		currentPage, err := pdfReader.GetPage(p + 1)
		if err != nil {
			return 0, 0, 0, nil, err
		}
		if p == 0 {
			pcx = currentPage.MediaBox.Height() * 0.98
			pcy = currentPage.MediaBox.Width() * 0.01
		}
		if err = pw.AddPage(currentPage); err != nil {
			return 0, 0, 0, nil, err
		}
	}
	return colPages, pcx, pcy, pdfReader, nil
}

func createOutlineItemWithTitle(title string, link, pcx, pcy float64, pdfReader *pdf.PdfReader) *pdf.OutlineItem {
	linkInt := max(int64(link), 0)

	oi := pdf.NewOutlineItem(title, pdf.NewOutlineDest(linkInt, pcy, pcx))

	currOI, err := pdfReader.GetOutlines()
	if err == nil {
		for _, v := range currOI.Items() {
			oi.Add(v)
		}
	}
	return oi
}

// writeOutput атомарно записывает PDF.
func writeOutput(pw *pdf.PdfWriter, outputFile string) error {
		return fileutil.WriteFileAtomic(outputFile, func(tmpFile string) (retErr error) {
			fo, err := os.Create(tmpFile)
			if err != nil {
				return err
			}
			defer func() {
				// Итоговая ошибка flush проверяется явным fo.Close() ниже; здесь —
				// только страховка закрытия на всех путях ошибки.
				if err := fo.Close(); err != nil && retErr == nil {
					retErr = fmt.Errorf("закрыть выходной файл: %w", err)
				}
			}()

			slog.Debug("Вывод файла", slog.String("file", tmpFile))
			if err := pw.Write(fo); err != nil {
				return err
			}
			return fo.Close()
		})
}

// CollectPdfFiles оставлен для обратной совместимости с toc/engine.
// Возвращает только .pdf файлы из директории.
func CollectPdfFiles(dir string) ([]string, error) {
	entries, err := CollectEntries(dir, false)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.Kind != KindDivider && e.FullPath != "" {
			files = append(files, e.FullPath)
		}
	}
	return files, nil
}

// appendixInfoFromOutline читает outline PDF и возвращает карту
// номер_страницы(1-based) → буква_приложения.
// Используется в pnpdf для определения на каких страницах рисовать "Приложение X".
func AppendixInfoFromOutline(pdfReader *pdf.PdfReader) map[int]string {
	result := make(map[int]string)
	outlines, err := pdfReader.GetOutlines()
	if err != nil || outlines == nil {
		slog.Debug("AppendixInfoFromOutline: no outlines found")
		return result
	}
	slog.Debug("AppendixInfoFromOutline", slog.Int("outlineItems", len(outlines.Items())))
	for _, item := range outlines.Items() {
		title := item.Title
		slog.Debug("outline item", slog.String("title", title))
		if !strings.HasPrefix(title, "Приложение ") {
			continue
		}
		// "Приложение А. Название" → буква "А"
		rest := strings.TrimPrefix(title, "Приложение ")
		before, _, ok := strings.Cut(rest, ".")
		if !ok {
			continue
		}
		letter := strings.TrimSpace(before)
		// OutlineDest — struct (не pointer), всегда ненулевой.
		// страница 0-based → +1 для 1-based
		page := int(item.Dest.Page) + 1
		result[page] = letter
		slog.Debug("appendix outline entry",
			slog.Int("page", page),
			slog.String("letter", letter))
	}
	slog.Debug("AppendixInfoFromOutline done", slog.Int("entries", len(result)))
	return result
}
