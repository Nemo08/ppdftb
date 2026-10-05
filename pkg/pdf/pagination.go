package pdf

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"log/slog"

	c "github.com/oliverpool/unipdf/v3/creator"
	pdf "github.com/oliverpool/unipdf/v3/model"
	"github.com/oliverpool/unipdf/v3/model/optimize"

	"github.com/Nemo08/ppdftb/pkg/fontutil"
)

// A4 in points (72 dpi): 210×297 mm ≈ 595×842 pt.
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

// Кегль и разрядка номера страницы — параметры исходной вёрстки, не трогаем.
const (
	numberFontSize  = 12.0
	numberCharSpace = 0.015 * 72 / 25.4 // трекинг в пунктах
)

// numberFont возвращает шрифт нумерации. Arial ищется в системных каталогах
// через fontutil.Resolve: если его нет или он не покрывает текст, подбирается
// любой покрывающий TrueType с предупреждением. Ошибка — только когда не
// нашлось ни одного подходящего шрифта (не подставляем pdf.DefaultFont: у
// Helvetica нет кириллических глифов, сборка падает на «unsupported text glyph»).
func numberFont(text string) (*pdf.PdfFont, error) {
	f, file, err := fontutil.Resolve("Arial", text)
	if err != nil {
		return nil, fmt.Errorf("шрифт нумерации: %w", err)
	}
	slog.Debug("шрифт нумерации", slog.String("base", f.BaseFont()), slog.String("file", file))
	return f, nil
}

// numberMetrics — метрики отрисовки номерного блока: кегль, трекинг и правила
// подсчёта ширины. Разрядка применяется ко всем символам, кроме цифр, —
// как в исходной вёрстке.
type numberMetrics struct {
	font      *pdf.PdfFont
	fontSize  float64
	charSpace float64
}

// isDigit — цифра номера страницы (для неё трекинг не применяется).
func isDigit(r rune) bool { return r >= '0' && r <= '9' }

// advance возвращает ширину руны в пунктах с учётом трекинга.
func (m numberMetrics) advance(r rune) float64 {
	w := 500.0
	if cm, ok := m.font.GetRuneMetrics(r); ok {
		w = cm.Wx * m.fontSize / 1000
	}
	if isDigit(r) {
		return w
	}
	return w + m.charSpace
}

// width возвращает суммарную ширину строки в пунктах.
func (m numberMetrics) width(s string) float64 {
	var w float64
	for _, r := range s {
		w += m.advance(r)
	}
	return w
}

// paragraph собирает абзац одного поля номерного блока (надпись приложения
// либо номер страницы). Символы раскладываются в отдельные chunks — это даёт
// каждому символу собственный трекинг, как в исходной вёрстке.
func (m numberMetrics) paragraph(cr *c.Creator, text string) *c.StyledParagraph {
	sp := cr.NewStyledParagraph()
	sp.SetEnableWrap(false)
	charSpacingTc := m.charSpace * 1000 / m.fontSize
	for i, r := range []rune(text) {
		var chunk *c.TextChunk
		if i == 0 {
			chunk = sp.SetText(string(r))
		} else {
			chunk = sp.Append(string(r))
		}
		chunk.Style.Font = m.font
		chunk.Style.FontSize = m.fontSize
		chunk.Style.Color = c.ColorRGBFrom8bit(0, 0, 0)
		if !isDigit(r) {
			chunk.Style.CharSpacing = charSpacingTc
		}
	}
	return sp
}

func addPageNumber(cr *c.Creator, page *pdf.PdfPage, pageNum, pf, nf int, prefix string) error {
	delta := nf - pf
	if pageNum < pf {
		return nil
	}

	// Надпись приложения и номер рисуются отдельными полями, разделитель «. »
	// остаётся в надписи — общий габарит блока и правый край прежние.
	label, num := prefix, strconv.Itoa(pageNum+delta)
	if label != "" {
		label += ". "
	}

	font, err := numberFont(label + num)
	if err != nil {
		return err
	}
	m := numberMetrics{font: font, fontSize: numberFontSize, charSpace: numberCharSpace}

	labelW := m.width(label)
	blockW := labelW + m.width(num)
	rightX := math.RoundToEven(cr.Context().PageWidth - Mm2px(8))

	// Левый край блока: правый край не меняем. В альбомной ориентации добавка
	// Mm2px(40) — калиброванный сдвиг под поле номера на бланке.
	x0 := rightX - blockW
	landscape := isA4Landscape(page)
	if landscape {
		x0 += Mm2px(40)
	}

	draw := func(text string, x float64) error {
		if text == "" {
			return nil
		}
		sp := m.paragraph(cr, text)
		if landscape {
			sp.SetAngle(-90)
			sp.SetPos(x, cr.Context().PageHeight-Mm2px(52.2))
		} else {
			sp.SetPos(x, Mm2px(5.2))
		}
		return cr.Draw(sp)
	}

	if err := draw(label, x0); err != nil {
		return err
	}
	return draw(num, x0+labelW)
}

// PaginationOptions — настройки нумерации.
type PaginationOptions struct {
	// Appendix включает автодетект приложений из outline PDF.
	// На страницах приложений перед номером рисуется "Приложение А".
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

	// Закладки необязательны: при ошибке останемся без дерева (nil обрабатывается ниже).
	outlineTree, err := pdfReader.GetOutlines()
	if err != nil {
		slog.Debug("не удалось прочитать закладки", slog.String("err", err.Error()))
	}
	cr := c.New()

	slog.Debug("MakePagination",
		slog.Int("totalPages", colPages),
		slog.Int("pageFrom", pf),
		slog.Int("numberFrom", nf),
		slog.Bool("appendix", o.Appendix))

	cfg := pageNumberConfig{
		pageFrom:    pf,
		numberFrom:  nf,
		appendix:    o.Appendix,
		appendixMap: appendixMap,
	}
	for p := range colPages {
		if err := addNumberedPage(ctx, pdfReader, cr, p+1, cfg); err != nil {
			return err
		}
	}

	if outlineTree != nil {
		cr.SetOutlineTree(outlineTree.ToOutlineTree())
	}
	return writePdf(cr, ofn)
}

// pageNumberConfig описывает параметры нумерации страниц.
type pageNumberConfig struct {
	pageFrom    int            // номер, с которого начинается печать страниц
	numberFrom  int            // номер, с которого начинается отсчёт номеров страниц
	appendix    bool           // режим приложений (надпись "Приложение X" перед номером)
	appendixMap map[int]string // страница → префикс приложения
}

// addNumberedPage добавляет страницу в creator и рисует на ней номер
// (с префиксом приложения при включённом режиме приложений).
func addNumberedPage(ctx context.Context, pdfReader *pdf.PdfReader, cr *c.Creator, pageNum int, cfg pageNumberConfig) error {
	currentPage, err := fetchAndAddPage(ctx, pdfReader, cr, pageNum)
	if err != nil {
		return err
	}
	return drawPageNumber(ctx, cr, currentPage, pageNum, cfg)
}

// fetchAndAddPage извлекает страницу из pdfReader и добавляет её в creator.
func fetchAndAddPage(ctx context.Context, pdfReader *pdf.PdfReader, cr *c.Creator, pageNum int) (*pdf.PdfPage, error) {
	currentPage, err := pdfReader.GetPage(pageNum)
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return nil, err
	}
	if err := cr.AddPage(currentPage); err != nil {
		slog.ErrorContext(ctx, err.Error())
		return nil, err
	}
	return currentPage, nil
}

// drawPageNumber вычисляет префикс приложения и рисует номер на странице.
func drawPageNumber(ctx context.Context, cr *c.Creator, currentPage *pdf.PdfPage, pageNum int, cfg pageNumberConfig) error {
	prefix := ""
	if cfg.appendix {
		prefix = appendixPrefix(pageNum, cfg.appendixMap)
	}
	if prefix != "" {
		slog.Debug("appendix page number",
			slog.Int("page", pageNum),
			slog.String("prefix", prefix))
	}
	if err := addPageNumber(cr, currentPage, pageNum, cfg.pageFrom, cfg.numberFrom, prefix); err != nil {
		slog.ErrorContext(ctx, err.Error())
		return err
	}
	return nil
}

// appendixRange — диапазон страниц приложения в outline PDF.
type appendixRange struct {
	fromPage int // 1-based включительно
	letter   string
}

// buildAppendixPageMap строит карту страница(1-based) → "Приложение А"
// на основе outline PDF сформированного mpdf с флагом -appendix.
// Каждое приложение занимает страницы от своей закладки до следующей.
func buildAppendixPageMap(pdfReader *pdf.PdfReader) map[int]string {
	result := make(map[int]string)

	outlines, err := pdfReader.GetOutlines()
	if err != nil || outlines == nil {
		return result
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
		before, _, ok := strings.Cut(rest, ".")
		if !ok {
			continue
		}
		letter := strings.TrimSpace(before)
		ranges = append(ranges, appendixRange{fromPage: page, letter: letter})
	}

	// Кол-во страниц для вспомогательной карты; при сбое 0 корректно отсечётся в appendixRangeEnd.
	totalPages, err := pdfReader.GetNumPages()
	if err != nil {
		slog.Debug("не удалось получить число страниц", slog.String("err", err.Error()))
	}
	slog.Debug("buildAppendixPageMap",
		slog.Int("outlineItems", len(outlines.Items())),
		slog.Int("ranges", len(ranges)),
		slog.Int("totalPages", totalPages))

	// Для каждого диапазона заполняем карту до следующей закладки.
	for i, r := range ranges {
		toPage := appendixRangeEnd(ranges, pages, i, totalPages)
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

// appendixRangeEnd определяет последнюю страницу диапазона приложения:
// страница следующей закладки верхнего уровня минус один (или конец PDF).
func appendixRangeEnd(ranges []appendixRange, pages []int, i, totalPages int) int {
	// Если есть следующее приложение — его первая страница граница текущего.
	if i+1 < len(ranges) {
		return ranges[i+1].fromPage - 1
	}
	// Ищем ближайшую закладку верхнего уровня после r.fromPage.
	r := ranges[i]
	for _, p := range pages {
		if p > r.fromPage {
			return p - 1
		}
	}
	return totalPages
}

// appendixPrefix возвращает префикс для страницы или пустую строку.
func appendixPrefix(page int, appendixMap map[int]string) string {
	if appendixMap == nil {
		return ""
	}
	return appendixMap[page]
}

func openPdfReader(ifn string) (reader *pdf.PdfReader, closer func(), err error) {
	if _, err := os.Stat(ifn); err != nil {
		return nil, nil, err
	}
	//nolint:gosec // чтение PDF по пути из CLI-аргумента — ожидаемое поведение
	data, err := os.Open(ifn)
	if err != nil {
		return nil, nil, err
	}
	closeLogged := func() {
		if err := data.Close(); err != nil {
			slog.Debug("не удалось закрыть PDF", slog.String("file", ifn), slog.String("err", err.Error()))
		}
	}
	reader, err = pdf.NewPdfReader(data)
	if err != nil {
		closeLogged()
		return nil, nil, err
	}
	return reader, closeLogged, nil
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
