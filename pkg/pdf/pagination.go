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

// pageRotation возвращает поворот страницы /Rotate, нормализованный до
// 0/90/180/270. Поворот, не кратный 90, считаем некорректным и трактуем как 0.
func pageRotation(page *pdf.PdfPage) int64 {
	if page == nil || page.Rotate == nil {
		return 0
	}
	r := *page.Rotate % 360
	if r < 0 {
		r += 360
	}
	switch r {
	case 90, 180, 270:
		return r
	default:
		return 0
	}
}

// effectivePageSize возвращает видимый размер листа — MediaBox со сменой места
// ширины и высоты при /Rotate 90/270. Именно так страницу показывает ридер,
// и именно на эти размеры должна ориентироваться нумерация.
func effectivePageSize(page *pdf.PdfPage) (w, h float64) {
	if page == nil || page.MediaBox == nil {
		return 0, 0
	}
	w, h = page.MediaBox.Width(), page.MediaBox.Height()
	if r := pageRotation(page); r == 90 || r == 270 {
		w, h = h, w
	}
	return w, h
}

// isA4Landscape определяет альбомный А4 по видимому размеру листа (с учётом
// /Rotate). Альбомная страница получается двумя способами — расширением
// MediaBox (842×595) или поворотом портретного MediaBox (595×842 + /Rotate
// 90/270), и effectivePageSize сводит оба случая к одному размеру. Нестандартные
// длинные листы (например, план AutoCAD: MediaBox 842×2381 + /Rotate 270)
// альбомным А4 не считаются, поэтому номер на них не поворачивается.
func isA4Landscape(page *pdf.PdfPage) bool {
	w, h := effectivePageSize(page)
	return w > h && math.Abs(w-a4LandscapeW) < a4Tol && math.Abs(h-a4LandscapeH) < a4Tol
}

// displayAngle возвращает видимый угол номера: -90 на альбомном А4 (надпись
// идёт вдоль длинной стороны листа), 0 во всех остальных случаях.
func displayAngle(landscape bool) float64 {
	if landscape {
		return -90
	}
	return 0
}

// displayToPage переводит точку из display-координат (лист так, как его
// показывает ридер; X вправо, Y вниз от верхнего края) в координаты MediaBox
// страницы. При /Rotate 0 перенос тождественный. w и h — ширина и высота
// MediaBox.
//
// Матрицы выведены из смещения углов листа при повороте на /Rotate по часовой:
// 90 → (x, y) ↦ (y, h−x), 180 → (w−x, h−y), 270 → (w−y, x).
func displayToPage(rot int64, w, h, x, y float64) (px, py float64) {
	switch rot {
	case 90:
		return y, h - x
	case 180:
		return w - x, h - y
	case 270:
		return w - y, x
	default:
		return x, y
	}
}

// pageAngle считает угол отрисовки текста в координатах MediaBox из видимого
// угла (displayAngle). К видимому углу ПРИБАВЛЯЕТСЯ поворот страницы /Rotate:
// creator рисует текст во flipped-кадре (ось Y вниз), где RotateDeg применяется
// с обратным знаком, и именно сложение даёт правильную ориентацию номера в
// ридере. Проверено на выгрузке AutoCAD (MediaBox 842×2381 + /Rotate 270):
// SetAngle(90) давал перевёрнутый номер (dir (0,-1) в PDF), а -90 — правильный
// горизонтальный (dir (0,1), как у штампов чертежа).
func pageAngle(angleDeg float64, rot int64) float64 {
	a := math.Mod(angleDeg+float64(rot), 360)
	switch {
	case a <= -180:
		return a + 360
	case a > 180:
		return a - 360
	default:
		return a
	}
}

// Кегль номера страницы — параметр исходной вёрстки, не трогаем.
const (
	numberFontSize = 12.0

	// appendixLabelShiftMM — сдвиг надписи приложения влево от номера, мм.
	appendixLabelShiftMM = 5.0
)

// Базовые отступы номерного блока от края страницы, мм. Откалиброваны по
// исходной вёрстке и включают уже сделанный сдвиг на 2 мм вниз:
//
//	numberTopMM    = 5.2 + 2.0  (портрет, отсчёт от верхнего края)
//	numberBottomMM = 52.2 - 2.0 (ландшафт, отсчёт от нижнего края)
const (
	numberTopMM    = 7.2
	numberBottomMM = 50.2
)

// Подстройка номерного блока относительно этих баз, мм. Двигает вместе и
// надпись приложения, и сам номер: draw двигает оба от одной базовой линии,
// иначе между ними появляется перепад.
//
// Знак у ландшафта отрицательный: положительное значение увеличило бы отступ
// от нижнего края и ушло бы вниз по странице, а нужно вверх. В портрете
// отступ отсчитывается от верхнего края, поэтому подъём — это вычитание
// (ось Y в creator направлена вниз, см. baseY ниже).
const (
	numberRaisePortraitMM  = 1.0
	numberRaiseLandscapeMM = -3.0
)

// numberFontFamily — семейство шрифта нумерации. Вынесено в константу, потому
// что на него ссылается installFontWidthRepair в логе.
const numberFontFamily = "Arial"

// numberFont возвращает шрифт нумерации. Arial ищется в системных каталогах
// через fontutil.Resolve: если его нет или он не покрывает текст, подбирается
// любой покрывающий TrueType с предупреждением. Ошибка — только когда не
// нашлось ни одного подходящего шрифта (не подставляем pdf.DefaultFont: у
// Helvetica нет кириллических глифов, сборка падает на «unsupported text glyph»).
func numberFont(text string) (*pdf.PdfFont, error) {
	f, file, err := fontutil.Resolve(numberFontFamily, text)
	if err != nil {
		return nil, fmt.Errorf("шрифт нумерации: %w", err)
	}
	slog.Debug("шрифт нумерации", slog.String("base", f.BaseFont()), slog.String("file", file))
	return f, nil
}

// numberMetrics — метрики отрисовки номерного блока: шрифт и кегль. Ширина
// строки и абзац отдаются в fontutil и creator, здесь остаётся только пара
// «шрифт + кегль» и правило их применения к одному полю блока.
//
// Разрядка (трекинг) намеренно не применяется. unipdf переводит
// TextChunk.Style.CharSpacing в оператор Tc, из-за чего надпись в файле
// разъезжается примерно на 1 pt на символ, а ширина, по которой мы
// ставим номер страницы, считается без этой правки. Итог: номер
// страницы уезжал внутрь надписи «Приложение А» и они сливались в один
// блок. Трекинг 0.015 мм визуально неразличим, поэтому убираем его.
type numberMetrics struct {
	font     *pdf.PdfFont
	fontSize float64
}

// width возвращает суммарную ширину строки в пунктах. Сами авансы берёт
// fontutil: это точная метрика шрифта, а не политика нумерации.
func (m numberMetrics) width(s string) float64 {
	return fontutil.StringWidth(m.font, s, m.fontSize)
}

// paragraph собирает абзац одного поля номерного блока (надпись приложения
// либо номер страницы).
//
// Текст кладём одним chunk: разбиение на chunks заставляет unipdf заводить
// отдельный ресурс шрифта на каждый фрагмент и вставлять компенсирующие
// TJ-поправки, из-за чего интервалы выглядят рваными.
func (m numberMetrics) paragraph(cr *c.Creator, text string) *c.StyledParagraph {
	sp := cr.NewStyledParagraph()
	sp.SetEnableWrap(false)

	chunk := sp.SetText(text)
	chunk.Style.Font = m.font
	chunk.Style.FontSize = m.fontSize
	chunk.Style.Color = c.ColorRGBFrom8bit(0, 0, 0)
	return sp
}

// numberLayout — геометрия номерного блока в display-координатах: левый край
// надписи приложения, базовая линия блока и начало номера.
type numberLayout struct {
	labelX float64
	baseY  float64
	numX   float64
	numY   float64
}

// layoutNumberBlock раскладывает номерной блок по видимой ширине и высоте
// листа (с учётом /Rotate). Правила исходной вёрстки: в портрете блок
// упирается в правое поле 8 мм, в альбомном А4 поверх него добавляется
// калиброванный сдвиг под поле номера на бланке (сам бланк повёрнут, поэтому
// блок уводится вправо почти к самому краю MediaBox, зазор около 0.6 мм).
//
// Ось Y направлена вниз от верхнего края листа (см. block.go в unipdf/creator:
// cc.Translate(blk.xPos, ctx.PageHeight-blk.yPos-blkHeight)). Портрет: отступ
// от верхнего края. Ландшафт: отступ от нижнего края — y = высота − отступ.
func layoutNumberBlock(labelW, numW, effW, effH float64, landscape bool) numberLayout {
	blockW := labelW + numW
	rightX := math.RoundToEven(effW - Mm2px(8))

	// Левый край блока.
	x0 := rightX - blockW
	if landscape {
		x0 += Mm2px(45)
	}

	// Надпись приложения сдвинута влево от номера, чтобы разнести их на бланке.
	labelX := x0 - Mm2px(appendixLabelShiftMM)

	baseY := Mm2px(numberTopMM - numberRaisePortraitMM)
	numX := x0 + labelW
	numY := baseY
	if landscape {
		baseY = effH - Mm2px(numberBottomMM+numberRaiseLandscapeMM)
		// В ландшафте текст повёрнут на -90°, направление чтения становится по Y
		// (вниз), и «после надписи» — это «под надписью». Прибавлять labelW к X
		// нельзя: номер уходит за правый край страницы и становится невидимым.
		numX = labelX
		numY = baseY + labelW
	}

	return numberLayout{labelX: labelX, baseY: baseY, numX: numX, numY: numY}
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
	m := numberMetrics{font: font, fontSize: numberFontSize}

	// Геометрия и угол считаются в display-координатах — лист так, как его
	// показывает ридер, с учётом /Rotate. Причина: на страницах с /Rotate
	// 90/270 MediaBox остаётся портретным, и номер, нарисованный «сверху»
	// MediaBox, в ридере оказывается повёрнутым. Именно это давали выгрузки
	// AutoCAD — план 44. План М1_500.pdf (MediaBox 842×2381 + /Rotate 270)
	// показывался горизонтально, а номер стоял повёрнутым у левого края.
	// Позиции переносятся в координаты MediaBox через displayToPage, а из
	// видимого угла вычитается поворот страницы (pageAngle). При /Rotate 0
	// переносы тождественны, поведение неповёрнутых страниц не меняется.
	rot := pageRotation(page)
	pageW, pageH := page.MediaBox.Width(), page.MediaBox.Height()
	effW, effH := effectivePageSize(page)
	landscape := isA4Landscape(page)

	lay := layoutNumberBlock(m.width(label), m.width(num), effW, effH, landscape)

	// Компенсация на страницах с /Rotate 90/270 (выгрузки AutoCAD). unipdf
	// поворачивает блок номерного поля вокруг его центра (см. block.go в
	// creator: Translate(w/2, h/2); RotateDeg; Translate(-w/2, -h/2)), из-за
	// чего на повёрнутых листах надпись прижимается к углу на ~4 мм ближе,
	// чем на неповёрнутых. Сдвигаем видимую позицию блока вниз и влево на
	// эти 4 мм, отступы от краёв совпадают с обычными листами. Альбомного
	// А4 это не касается — там поворот выставляется своим полем layout'a.
	if rot == 90 || rot == 270 {
		lay.labelX -= Mm2px(4)
		lay.numX -= Mm2px(4)
		lay.baseY += Mm2px(4)
		lay.numY += Mm2px(4)
	}

	angle := pageAngle(displayAngle(landscape), rot)

	draw := func(text string, dx, dy float64) error {
		if text == "" {
			return nil
		}
		sp := m.paragraph(cr, text)
		sp.SetAngle(angle)
		px, py := displayToPage(rot, pageW, pageH, dx, dy)
		sp.SetPos(px, py)
		return cr.Draw(sp)
	}

	if err := draw(label, lay.labelX, lay.baseY); err != nil {
		return err
	}
	return draw(num, lay.numX, lay.numY)
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
	// Страницы держим в своём срезе: creator материализует блоки и собирает
	// объекты шрифтов только при записи, и до этого момента шрифта нумерации в
	// ресурсах страницы нет — чинить метрики раньше нечего.
	pages := make([]*pdf.PdfPage, 0, colPages)
	for p := range colPages {
		page, err := addNumberedPage(ctx, pdfReader, cr, p+1, cfg)
		if err != nil {
			return err
		}
		pages = append(pages, page)
	}
	installFontWidthRepair(cr, pages)

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

// addNumberedPage добавляет страницу в creator, рисует на ней номер (с префиксом
// приложения при включённом режиме приложений) и возвращает саму страницу —
// её ресурсы понадобятся ремонту метрик шрифта (см. installFontWidthRepair).
func addNumberedPage(ctx context.Context, pdfReader *pdf.PdfReader, cr *c.Creator, pageNum int, cfg pageNumberConfig) (*pdf.PdfPage, error) {
	currentPage, err := fetchAndAddPage(ctx, pdfReader, cr, pageNum)
	if err != nil {
		return nil, err
	}
	if err := drawPageNumber(ctx, cr, currentPage, pageNum, cfg); err != nil {
		return nil, err
	}
	return currentPage, nil
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

// installFontWidthRepair вешает пересчёт метрик составных шрифтов на хук
// доступа к PdfWriter.
//
// ЧТО делает. Заменяет /W и /DW у всех составных шрифтов (Type0 +
// CIDFontType2) на страницах на значения, вычисленные по авансам глифов из
// внедрённого FontFile2 (таблица hmtx). Ничего не субсетит и не меняет
// кодировку — только метрики.
//
// ЗАЧЕМ. unipdf собирает /W в makeCIDWidthArr (model/font_composite.go): берёт
// руны, отсортированные по Unicode, склеивает подряд идущие с одинаковой
// шириной в один диапазон и ставит границы диапазона ПО GID. Склейка корректна
// только если GID идут по рунам строго по возрастанию. В реальных шрифтах
// (Arial) это не так: между GID первой и последней руны диапазона лежат глифы
// ДРУГИХ рун, и unipdf затирает их ширины одним значением. В нашем томе на
// 49 страницах это давало 56 битых диапазонов, суммарно 24 593 CID, включая
// диапазоны нулевой ширины. В Acrobat надпись «Приложение А» наезжала сама на
// себя: глиф «И» получал /W = 354 вместо настоящих 718.8, то есть 4.25 pt
// вместо 8.63 pt при кегле 12 pt.
//
// ПОЧЕМУ ЭТО ЛОВИТСЯ ТОЛЬКО В ACROBAT. Ширины глифов есть в двух местах:
// в /W и во внедрённом шрифте (hmtx). Acrobat, сжимая файл, доверяет /W.
// PyMuPDF и большинство других рендерерок меряют по hmtx. Поэтому на них файл
// выглядит правильным, и проверить правку глазами через рендер нельзя — нужно
// сверять /W с hmtx напрямую (см. CidWidthRepair.CountBrokenOnPage и регрессионные
// тесты в pkg/pdf/cidwidths_test.go).
//
// ПОЧЕМУ ХУК, А НЕ ПОЧИНКА СРАЗУ ПОСЛЕ ОТРИСОВКИ. creator материализует блоки и
// создаёт объекты шрифтов только при записи: paragraph.go вызывает ToPdfObject()
// при слиянии ресурсов блока в ресурсы страницы, а это происходит уже после
// того, как Draw вернулся. Проверено тестом: сразу после отрисовки в
// ресурсах страницы нет ни одного составного шрифта, а к моменту записи их
// 3–5 на страницу. Поэтому и нужен хук: SetPdfWriterAccessFunc вызывается
// после Finalize, когда блоки уже слиты и шрифты созданы, но до
// pdfWriter.AddPage и сериализации. Это единственное окно, где объект шрифта
// уже существует, а файл ещё не записан.
//
// ПОЧЕМУ НЕ ВТОРОЙ ПРОХОД ПО ФАЙЛУ. Ремонт по готовому файлу (прочитать,
// распарсить, переписать через новый PdfWriter) тоже работает, но он читает и
// записывает весь том второй раз, держит его целиком в памяти, а новый
// PdfWriter не наследует каталог документа: закладки и метки страниц пришлось
// бы переносить вручную (при первом варианте они молча терялись — 106 закладок
// превращались в 0). Через хук том собирается за один проход, и закладки
// остаются заботой creator.
//
// Кэш разобранных FontFile2 общий на весь том, иначе Arial (1 МБ) читался бы
// заново на каждой странице.
func installFontWidthRepair(cr *c.Creator, pages []*pdf.PdfPage) {
	// Один ремонт на весь том: внутри кэш по FontFile2.
	repair := fontutil.NewCidWidthRepair()

	cr.SetPdfWriterAccessFunc(func(_ *pdf.PdfWriter) error {
		// Страницы лежат в ресурсах уже готовыми к сериализации, правим /W
		// на месте — до того, как writer превратит их в объекты файла.
		for i, page := range pages {
			// RepairPage трогает только Type0/CIDFontType2 с
			// CIDToGIDMap /Identity; простые шрифты и шрифты с
			// CIDToGIDStream пропускает (индекс /W там — не GID).
			if _, err := repair.RepairPage(page); err != nil {
				// Ошибку не глотаем: молчаливый выход даст битый /W, а
				// пользователь увидит наезд букв без объяснения.
				return fmt.Errorf("repair page %d font widths: %w", i+1, err)
			}
		}
		slog.Debug("Метрики составных шрифтов пересчитаны",
			slog.Int("страниц", len(pages)),
			slog.String("шрифт", numberFontFamily))
		return nil
	})
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
