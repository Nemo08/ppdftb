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

// MakePagination добавляет нумерацию страниц в готовый pdf файл ifn, начиная со
// страницы pf c начальным номером nf и записывает в файл ofn
func MakePagination(ctx context.Context, ifn, ofn string, pf, nf int) error {
	if _, err := os.Stat(ifn); err != nil {
		slog.ErrorContext(ctx, err.Error())
		return err
	}

	data, err := os.Open(ifn)
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return err
	}
	defer data.Close()

	//Создаем читалку pdf
	pdfReader, err := pdf.NewPdfReader(data)
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return err
	}

	//Получаем количество страниц в файле
	colPages, err := pdfReader.GetNumPages()
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return err
	}

	//Получаем закладки
	outlineTree, err := pdfReader.GetOutlines()

	var currentPage *pdf.PdfPage

	//Создаем pdf creator
	cr := c.New()

	//Проходим по страницам
	for p := 0; p < colPages; p++ {
		currentPage, err = pdfReader.GetPage(p + 1)
		if err != nil {
			slog.ErrorContext(ctx, err.Error())
			return err
		}

		//Добавляем страницу в creator
		err = cr.AddPage(currentPage)
		if err != nil {
			slog.ErrorContext(ctx, err.Error())
			return err
		}
		delta := nf - pf

		w := math.RoundToEven(cr.Context().PageWidth - Mm2px(10))
		if p+1 >= int(pf) {
			para := c.Paragraph{}
			para.SetFont(pdf.DefaultFont())
			para.SetFontSize(12)

			para.SetColor(c.ColorRGBFrom8bit(0, 0, 0))
			para.SetText(fmt.Sprintf("%v", p+1+delta))

			landscape := isA4Landscape(currentPage)
			if landscape {
				// А4 горизонтальная: номер в правый нижний угол, повёрнут на -90°.
				// После разворота страницы на 90° по часовой (брошюровка)
				// номер окажется в правом верхнем углу и будет читаться прямо.
				para.SetAngle(-90)
				para.SetPos(w-para.Width()/2, cr.Context().PageHeight-Mm2px(12.2))
			} else {
				para.SetPos(w-para.Width()/2, Mm2px(10.2))
			}
			cr.Draw(&para)
		}
	}
	//Вставляем закладки
	if err == nil && outlineTree != nil {
		cr.SetOutlineTree(outlineTree.ToOutlineTree())
	}
	cr.SetOptimizer(optimize.New(optimize.Options{
		CompressStreams:                 true,
		UseObjectStreams:                true,
		CombineDuplicateStreams:         true,
		CombineDuplicateDirectObjects:   true,
		CombineIdenticalIndirectObjects: true,
		ImageUpperPPI:                   300,
	}))
	err = cr.WriteToFile(ofn)
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return err
	}

	return nil
}


