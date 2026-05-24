package pdf

import (
	"context"
	"fmt"
	"math"
	"os"

	"log/slog"

	c "github.com/oliverpool/unipdf/v3/creator"
	pdf "github.com/oliverpool/unipdf/v3/model"
	"github.com/oliverpool/unipdf/v3/model/optimize"
)

// A4 in points (72 dpi): 210×297 mm ≈ 595×842 pt
const a4LandscapeW = 842.0
const a4LandscapeH = 595.0
const a4Tol = 5.0 // допуск на неточный MediaBox

func isA4Landscape(page *pdf.PdfPage) bool {
	if page.MediaBox == nil {
		return false
	}
	w := page.MediaBox.Width()
	h := page.MediaBox.Height()

	// Явно ландшафтный MediaBox (Word → PDF).
	if w > h && math.Abs(w-a4LandscapeW) < a4Tol && math.Abs(h-a4LandscapeH) < a4Tol {
		return true
	}

	// Портретный MediaBox + флаг Rotate 90/270 (некоторые генераторы PDF).
	var rotate int64
	if page.Rotate != nil {
		rotate = *page.Rotate
	}
	if (rotate == 90 || rotate == 270) &&
		math.Abs(h-a4LandscapeW) < a4Tol && math.Abs(w-a4LandscapeH) < a4Tol {
		return true
	}

	return false
}

func addPageNumber(cr *c.Creator, page *pdf.PdfPage, pageNum, pf, nf int) {
	delta := nf - pf
	if pageNum < pf {
		return
	}
	para := c.Paragraph{}
	para.SetFont(pdf.DefaultFont())
	para.SetFontSize(12)
	para.SetColor(c.ColorRGBFrom8bit(0, 0, 0))
	para.SetText(fmt.Sprintf("%v", pageNum+delta))

	w := math.RoundToEven(cr.Context().PageWidth - Mm2px(10))
	if isA4Landscape(page) {
		para.SetAngle(-90)
		para.SetPos(w-para.Width()/2, cr.Context().PageHeight-Mm2px(12.2))
	} else {
		para.SetPos(w-para.Width()/2, Mm2px(10.2))
	}
	cr.Draw(&para)
}

// MakePagination добавляет нумерацию страниц в готовый pdf файл ifn, начиная со
// страницы pf c начальным номером nf и записывает в файл ofn
func MakePagination(ctx context.Context, ifn, ofn string, pf, nf int) error {
	pdfReader, cleanup, err := openPdfReader(ifn)
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return err
	}
	defer cleanup()

	colPages, err := pdfReader.GetNumPages()
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return err
	}

	outlineTree, _ := pdfReader.GetOutlines()
	cr := c.New()

	for p := 0; p < colPages; p++ {
		currentPage, err := pdfReader.GetPage(p + 1)
		if err != nil {
			slog.ErrorContext(ctx, err.Error())
			return err
		}
		if err := cr.AddPage(currentPage); err != nil {
			slog.ErrorContext(ctx, err.Error())
			return err
		}
		addPageNumber(cr, currentPage, p+1, pf, nf)
	}

	if outlineTree != nil {
		cr.SetOutlineTree(outlineTree.ToOutlineTree())
	}
	return writePdf(cr, ofn)
}

func openPdfReader(ifn string) (*pdf.PdfReader, func(), error) {
	if _, err := os.Stat(ifn); err != nil {
		return nil, nil, err
	}
	data, err := os.Open(ifn)
	if err != nil {
		return nil, nil, err
	}
	reader, err := pdf.NewPdfReader(data)
	if err != nil {
		data.Close()
		return nil, nil, err
	}
	return reader, func() { data.Close() }, nil
}

func writePdf(cr *c.Creator, ofn string) error {
	cr.SetOptimizer(optimize.New(optimize.Options{
		CompressStreams:                 true,
		UseObjectStreams:                true,
		CombineDuplicateStreams:         true,
		CombineDuplicateDirectObjects:   true,
		CombineIdenticalIndirectObjects: true,
		ImageUpperPPI:                   300,
	}))
	return cr.WriteToFile(ofn)
}
