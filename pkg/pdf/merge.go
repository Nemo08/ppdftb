package pdf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"log/slog"

	"github.com/Nemo08/ppdftb/pkg/fileutil"
	"github.com/maruel/natural"
	pdf "github.com/oliverpool/unipdf/v3/model"
)

// Merge объединяет все PDF-файлы из sourceFolder в один outputFile.
// Файлы сортируются natural sort; каждый файл становится закладкой верхнего уровня,
// внутренние закладки (outline) подшиваются под неё.
func Merge(ctx context.Context, sourceFolder, outputFile string) error {
	fileList, err := CollectPdfFiles(sourceFolder)
	if err != nil {
		return err
	}
	if len(fileList) == 0 {
		slog.DebugContext(ctx, "В папке нет pdf файлов для объединения", slog.String("folder", sourceFolder))
		return nil
	}

	sort.Sort(natural.StringSlice(fileList))

	pw := pdf.NewPdfWriter()
	otree := pdf.NewOutline()
	if _, err := mergeFiles(ctx, fileList, &pw, otree); err != nil {
		return err
	}

	pw.AddOutlineTree(otree.ToOutlineTree())

	return writeOutput(&pw, outputFile)
}

func mergeOneFile(file string, pw *pdf.PdfWriter, otree *pdf.Outline, totalPages *int) error {
	colPages, pcx, pcy, pdfReader, err := readAndAddPages(file, pw)
	if err != nil {
		return fmt.Errorf("файл %q: %w", filepath.Base(file), err)
	}

	oi := createOutlineItem(file, float64(*totalPages), pcx, pcy, pdfReader)
	otree.Add(oi)
	*totalPages += colPages
	return nil
}

func readAndAddPages(file string, pw *pdf.PdfWriter) (int, float64, float64, *pdf.PdfReader, error) {
	data, err := os.Open(file)
	if err != nil {
		return 0, 0, 0, nil, err
	}
	defer data.Close()

	pdfReader, err := pdf.NewPdfReader(data)
	if err != nil {
		return 0, 0, 0, nil, err
	}

	colPages, err := pdfReader.GetNumPages()
	if err != nil {
		return 0, 0, 0, nil, err
	}

	var pcx, pcy float64
	for p := 0; p < colPages; p++ {
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

func createOutlineItem(file string, link, pcx, pcy float64, pdfReader *pdf.PdfReader) *pdf.OutlineItem {
	linkInt := int64(link)
	if linkInt < 0 {
		linkInt = 0
	}

	linkText := strings.TrimSuffix(filepath.Base(file), filepath.Ext(filepath.Base(file)))
	oi := pdf.NewOutlineItem(linkText, pdf.NewOutlineDest(linkInt, pcy, pcx))

	currOI, err := pdfReader.GetOutlines()
	if err == nil {
		for _, v := range currOI.Items() {
			oi.Add(v)
		}
	}
	return oi
}

func mergeFiles(ctx context.Context, fileList []string, pw *pdf.PdfWriter, otree *pdf.Outline) (int, error) {
	totalPages := 0
	for _, file := range fileList {
		if err := mergeOneFile(file, pw, otree, &totalPages); err != nil {
			slog.ErrorContext(ctx, err.Error())
			return 0, err
		}
	}
	return totalPages, nil
}

func writeOutput(pw *pdf.PdfWriter, outputFile string) error {
	return fileutil.WriteFileAtomic(outputFile, func(tmpFile string) error {
		fo, err := os.Create(tmpFile)
		if err != nil {
			return err
		}
		defer fo.Close()

		slog.Debug("Вывод файла", slog.String("file", tmpFile))
		if err := pw.Write(fo); err != nil {
			return err
		}

		return fo.Close()
	})
}
