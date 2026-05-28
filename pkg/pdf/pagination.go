package pdf

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"

	"log/slog"

	c "github.com/oliverpool/unipdf/v3/creator"
	pdf "github.com/oliverpool/unipdf/v3/model"
	"github.com/oliverpool/unipdf/v3/model/optimize"
)

// A4 in points (72 dpi): 210×297 mm ≈ 595×842 pt
const a4LandscapeW = 842.0
const a4LandscapeH = 595.0
const a4Tol = 5.0

func isA4Landscape(page *pdf.PdfPage) bool {
	if page.MediaBox == nil {
		return false
	}
	w := page.MediaBox.Width()
	h := page.MediaBox.Height()

	if w > h && math.Abs(w-a4LandscapeW) < a4Tol && math.Abs(h-a4LandscapeH) < a4Tol {
		return true
	}
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

var loadCyrillicFont = sync.OnceValue(func() *pdf.PdfFont {
	f, err := pdf.NewCompositePdfFontFromTTFFile("C:/Windows/Fonts/arial.ttf")
	if err != nil {
		slog.Warn("Times New Roman not loaded, using default font", slog.String("err", err.Error()))
		return pdf.DefaultFont()
	}
	return f
})

func addPageNumber(cr *c.Creator, page *pdf.PdfPage, pageNum, pf, nf int, prefix string) error {
	delta := nf - pf
	if pageNum < pf {
		return nil
	}

	text := fmt.Sprintf("%v", pageNum+delta)
	if prefix != "" {
		text = prefix + ". " + text
	}

	rightX := math.RoundToEven(cr.Context().PageWidth - Mm2px(8))

	font := loadCyrillicFont()
	fontSize := 12.0
	charSpacing := 0.015 * 72 / 25.4
	charSpacingTc := charSpacing * 1000 / fontSize

	sp := cr.NewStyledParagraph()
	sp.SetEnableWrap(false)

	var textW float64
	runes := []rune(text)
	for i, r := range runes {
		isDigit := r >= '0' && r <= '9'
		var chunk *c.TextChunk
		if i == 0 {
			chunk = sp.SetText(string(r))
		} else {
			chunk = sp.Append(string(r))
		}
		chunk.Style.Font = font
		chunk.Style.FontSize = fontSize
		chunk.Style.Color = c.ColorRGBFrom8bit(0, 0, 0)
		if !isDigit {
			chunk.Style.CharSpacing = charSpacingTc
		}

		m, ok := font.GetRuneMetrics(r)
		charW := 500.0
		if ok {
			charW = m.Wx * fontSize / 1000
		}
		textW += charW
		if !isDigit {
			textW += charSpacing
		}
	}

	if isA4Landscape(page) {
		sp.SetAngle(-90)
		y := cr.Context().PageHeight - Mm2px(52.2)
		sp.SetPos(rightX-textW+Mm2px(40), y)
	} else {
		y := Mm2px(5.2)
		sp.SetPos(rightX-textW, y)
	}
	return cr.Draw(sp)
}

// PaginationOptions — настройки нумерации.
type PaginationOptions struct {
	// Appendix включает автодетект приложений из outline PDF.
	// На страницах приложений перед номером рисуется "Прил. А".
	Appendix bool
}

// PaginationOption — функциональная опция для MakePagination.
type PaginationOption func(*PaginationOptions)

// WithPaginationAppendix включает режим приложений.
func WithPaginationAppendix() PaginationOption {
	return func(o *PaginationOptions) { o.Appendix = true }
}

// MakePagination добавляет нумерацию страниц в готовый PDF файл ifn,
// начиная со страницы pf c начальным номером nf и записывает в файл ofn.
func MakePagination(ctx context.Context, ifn, ofn string, pf, nf int, opts ...PaginationOption) error {
	o := &PaginationOptions{}
	for _, opt := range opts {
		opt(o)
	}

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

	// Карта страница → буква приложения (только при -appendix).
	var appendixMap map[int]string
	if o.Appendix {
		appendixMap = buildAppendixPageMap(pdfReader)
		slog.Debug("appendix map", slog.Int("entries", len(appendixMap)))
	}

	outlineTree, _ := pdfReader.GetOutlines()
	cr := c.New()

	slog.Debug("MakePagination",
		slog.Int("totalPages", colPages),
		slog.Int("pageFrom", pf),
		slog.Int("numberFrom", nf),
		slog.Bool("appendix", o.Appendix))

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

		prefix := ""
		if o.Appendix {
			prefix = appendixPrefix(p+1, appendixMap)
		}
		if prefix != "" {
			slog.Debug("appendix page number",
				slog.Int("page", p+1),
				slog.String("prefix", prefix))
		}
		if err := addPageNumber(cr, currentPage, p+1, pf, nf, prefix); err != nil {
			slog.ErrorContext(ctx, err.Error())
			return err
		}
	}

	if outlineTree != nil {
		cr.SetOutlineTree(outlineTree.ToOutlineTree())
	}
	return writePdf(cr, ofn)
}

// buildAppendixPageMap строит карту страница(1-based) → "Прил. А"
// на основе outline PDF сформированного mpdf с флагом -appendix.
// Каждое приложение занимает страницы от своей закладки до следующей.
func buildAppendixPageMap(pdfReader *pdf.PdfReader) map[int]string {
	result := make(map[int]string)

	outlines, err := pdfReader.GetOutlines()
	if err != nil || outlines == nil {
		return result
	}

	type appendixRange struct {
		fromPage int // 1-based включительно
		letter   string
	}
	var ranges []appendixRange
	var pages []int // страницы всех закладок верхнего уровня для определения конца

	for _, item := range outlines.Items() {
		// OutlineDest — struct (не pointer), всегда ненулевой.
		page := int(item.Dest.Page) + 1 // 0-based → 1-based
		pages = append(pages, page)

		title := item.Title
		slog.Debug("buildAppendixPageMap outline item",
			slog.String("title", title),
			slog.Int("page", page))
		if !strings.HasPrefix(title, "Приложение ") {
			continue
		}
		rest := strings.TrimPrefix(title, "Приложение ")
		dotIdx := strings.Index(rest, ".")
		if dotIdx < 0 {
			continue
		}
		letter := strings.TrimSpace(rest[:dotIdx])
		ranges = append(ranges, appendixRange{fromPage: page, letter: letter})
	}

	totalPages, _ := pdfReader.GetNumPages()
	slog.Debug("buildAppendixPageMap",
		slog.Int("outlineItems", len(outlines.Items())),
		slog.Int("ranges", len(ranges)),
		slog.Int("totalPages", totalPages))

	// Для каждого диапазона заполняем карту до следующей закладки.
	for i, r := range ranges {
		toPage := totalPages
		// Ищем следующую закладку верхнего уровня после r.fromPage.
		for _, p := range pages {
			if p > r.fromPage && (i+1 >= len(ranges) || p <= ranges[i+1].fromPage) {
				toPage = p - 1
				break
			}
		}
		if i+1 < len(ranges) {
			toPage = ranges[i+1].fromPage - 1
		}
		slog.Debug("appendix range",
			slog.String("letter", r.letter),
			slog.Int("fromPage", r.fromPage),
			slog.Int("toPage", toPage))
		for pg := r.fromPage; pg <= toPage; pg++ {
			result[pg] = "Приложение " + r.letter
		}
	}
	return result
}

// appendixPrefix возвращает префикс для страницы или пустую строку.
func appendixPrefix(page int, appendixMap map[int]string) string {
	if appendixMap == nil {
		return ""
	}
	return appendixMap[page]
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
		_ = data.Close()
		return nil, nil, err
	}
	return reader, func() { _ = data.Close() }, nil
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
