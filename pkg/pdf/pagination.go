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

// numberMetrics — метрики отрисовки номерного блока: кегль и правила
// подсчёта ширины.
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

// advance возвращает натуральную ширину руны в пунктах.
func (m numberMetrics) advance(r rune) float64 {
	if cm, ok := m.font.GetRuneMetrics(r); ok {
		return cm.Wx * m.fontSize / 1000
	}
	return 500.0 * m.fontSize / 1000
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

	labelW := m.width(label)
	blockW := labelW + m.width(num)
	rightX := math.RoundToEven(cr.Context().PageWidth - Mm2px(8))

	// Левый край блока. В портрете блок упирается в правое поле 8 мм, в
	// ландшафте поверх него добавляется калиброванный сдвиг под поле номера
	// на бланке: сам бланк повёрнут, поэтому блок уводится вправо почти к
	// самому краю MediaBox (зазор до края около 0.6 мм).
	x0 := rightX - blockW
	landscape := isA4Landscape(page)
	if landscape {
		x0 += Mm2px(45)
	}

	// Надпись приложения сдвинута влево от номера, чтобы разнести их на бланке.
	labelX := x0 - Mm2px(appendixLabelShiftMM)

	// Базовая линия блока. ВАЖНО: в unipdf/creator ось Y направлена ВНИЗ от
	// верхнего края (см. block.go: cc.Translate(blk.xPos,
	// ctx.PageHeight-blk.yPos-blkHeight) — координаты блока переворачиваются
	// при отрисовке). Поэтому «вниз по странице» = увеличить y.
	//
	// Портрет: y отсчитывается от верхнего края, подстройка вычитается.
	// Ландшафт: отступ отсчитывается от нижнего края, поэтому y считается
	// как PageHeight минус отступ.
	baseY := Mm2px(numberTopMM - numberRaisePortraitMM)
	if landscape {
		baseY = cr.Context().PageHeight - Mm2px(numberBottomMM+numberRaiseLandscapeMM)
	}

	draw := func(text string, x, y float64) error {
		if text == "" {
			return nil
		}
		sp := m.paragraph(cr, text)
		if landscape {
			sp.SetAngle(-90)
		}
		sp.SetPos(x, y)
		return cr.Draw(sp)
	}

	// Положение номера относительно надписи. В портрете направление чтения —
	// по X, номер идёт справа. В ландшафте текст повёрнут на -90°, направление
	// чтения становится по Y (вниз), и «после надписи» — это «под надписью».
	// Прибавлять labelW к X в ландшафте нельзя: номер уходит за правый край
	// страницы и становится невидимым (x0+labelW ~= 920 при ширине 842).
	numX, numY := x0+labelW, baseY
	if landscape {
		numX, numY = labelX, baseY+labelW
	}

	if err := draw(label, labelX, baseY); err != nil {
		return err
	}
	return draw(num, numX, numY)
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
