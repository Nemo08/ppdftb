package pdf

import (
	"context"
	"fmt"
	"math"
	"os"

	"golang.org/x/exp/slog"

	c "github.com/loxiouve/unipdf/v3/creator"
	pdf "github.com/loxiouve/unipdf/v3/model"
)

// MakePagination добавляет нумерацию страниц в готовый pdf файл ifn, начиная со
// страницы pf c начальным номером nf и записывает в файл ofn
func MakePagination(ctx context.Context, ifn, ofn string, pf, nf int) error {
	if _, err := os.Stat(ifn); err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	data, err := os.Open(ifn)
	defer data.Close()

	//Создаем читалку pdf
	pdfReader, err := pdf.NewPdfReader(data)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	//Получаем количество страниц в файле
	colPages, err := pdfReader.GetNumPages()

	//Получаем закладки
	outlineTree, err := pdfReader.GetOutlines()

	var currentPage *pdf.PdfPage

	//Создаем pdf creator
	cr := c.New()

	//Проходим по страницам
	for p := 0; p < colPages; p++ {
		currentPage, err = pdfReader.GetPage(p + 1)
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
			return err
		}
		//Добавляем страницу в creator
		err = cr.AddPage(currentPage)
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
			return err
		}
		delta := nf - pf

		/*
			Не работает с какого-то момента
				//Рисуем хидер
				cr.DrawHeader(func(block *c.Block, args c.HeaderFunctionArgs) {
					if args.PageNum >= int(pf) {
						para := c.Paragraph{}
						para.SetFont(pdf.DefaultFont())
						para.SetFontSize(12)
						//para.SetPos(math.RoundToEven(cr.Context().PageWidth-Mm2px(13)), Mm2px(10))
						para.SetPos(w, Mm2px(10))
						fmt.Println("W", w)
						para.SetColor(c.ColorRGBFrom8bit(0, 0, 0))
						para.SetText(fmt.Sprintf("%v", args.PageNum+delta))
						block.Draw(&para)
					}
				})
		*/
		w := math.RoundToEven(cr.Context().PageWidth - Mm2px(10))
		if p+1 >= int(pf) {
			para := c.Paragraph{}
			para.SetFont(pdf.DefaultFont())
			para.SetFontSize(12)

			para.SetColor(c.ColorRGBFrom8bit(0, 0, 0))
			para.SetText(fmt.Sprintf("%v", p+1+delta))
			para.SetPos(w-para.Width()/2, Mm2px(10.2))
			cr.Draw(&para)
		}
	}
	//Вставляем закладки
	cr.SetOutlineTree(outlineTree.ToOutlineTree())
	err = cr.WriteToFile(ofn)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	return nil
}
