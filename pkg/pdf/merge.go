package pdf

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"log/slog"

	"github.com/maruel/natural"
	pdf "github.com/oliverpool/unipdf/v3/model"
)

// Merge объединяет все PDF-файлы из sourceFolder в один outputFile.
// Файлы сортируются natural sort; каждый файл становится закладкой верхнего уровня,
// внутренние закладки (outline) подшиваются под неё.
func Merge(ctx context.Context, sourceFolder, outputFile string) error {
	fileList, err := collectPdfFiles(sourceFolder)
	if err != nil {
		return err
	}
	if len(fileList) == 0 {
		slog.DebugContext(ctx, "В папке нет pdf файлов для объединения", slog.String("folder", sourceFolder))
		return nil
	}

	if err := validateFiles(fileList); err != nil {
		return err
	}

	sort.Sort(natural.StringSlice(fileList))

	pw := pdf.NewPdfWriter()
	otree := pdf.NewOutline()
	totalPages, err := mergeFiles(ctx, fileList, &pw, otree)
	if err != nil {
		return err
	}
	_ = totalPages

	pw.AddOutlineTree(otree.ToOutlineTree())

	return writeOutput(&pw, outputFile)
}

func collectPdfFiles(sourceFolder string) ([]string, error) {
	files, err := os.ReadDir(sourceFolder)
	if err != nil {
		return nil, err
	}

	var fileList []string
	for _, file := range files {
		if !file.IsDir() && strings.ToLower(filepath.Ext(file.Name())) == ".pdf" {
			fileList = append(fileList, filepath.Join(sourceFolder, file.Name()))
		}
	}
	return fileList, nil
}

func validateFiles(fileList []string) error {
	for _, f := range fileList {
		r, err := os.Open(f)
		if err != nil {
			return fmt.Errorf("файл %q не читается: %w", f, err)
		}
		r.Close()
	}
	return nil
}

func mergeOneFile(file string, pw *pdf.PdfWriter, otree *pdf.Outline, totalPages *int) error {
	colPages, pcx, pcy, pdfReader, err := readAndAddPages(file, pw)
	if err != nil {
		return err
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
	var b [8]byte
	rand.Read(b[:])
	suffix := hex.EncodeToString(b[:])
	tmpFile := outputFile + "." + suffix + ".tmp"

	fo, err := os.Create(tmpFile)
	if err != nil {
		return err
	}
	defer fo.Close()

	slog.Debug("Вывод файла", slog.String("file", tmpFile))
	if err := pw.Write(fo); err != nil {
		os.Remove(tmpFile)
		return err
	}

	if err := fo.Close(); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("закрытие tmp-файла: %w", err)
	}

	if err := os.Rename(tmpFile, outputFile); err != nil {
		os.Remove(tmpFile)
		return err
	}

	return nil
}
