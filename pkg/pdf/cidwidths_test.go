package pdf

import (
	"os"
	"path/filepath"
	"testing"

	c "github.com/oliverpool/unipdf/v3/creator"
	pdf "github.com/oliverpool/unipdf/v3/model"

	"github.com/Nemo08/ppdftb/pkg/fontutil"
)

// TestNumberFontWidthsSurviveCreatorWrite — сквозная регрессия на баг unipdf
// makeCIDWidthArr: тот склеивает руны в диапазоны /W по границам GID и затирает
// ширины глифов, попавших внутрь диапазона. Acrobat берёт ширины из /W, поэтому
// надпись «Приложение А» в нём налезает сама на себя, а рендерилки на основе
// hmtx показывают корректную картину.
//
// Тест рисует номер через штатный addPageNumber, записывает PDF и требует, чтобы
// в готовом файле НИ ОДНОГО составного шрифта не содержало расхождения /W с
// внедрённым FontFile2.
func TestNumberFontWidthsSurviveCreatorWrite(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "numbered.pdf")

	cr := c.New()
	page := pdf.NewPdfPage()
	page.MediaBox = &pdf.PdfRectangle{Urx: a4LandscapeW, Ury: a4LandscapeH}
	if err := cr.AddPage(page); err != nil {
		t.Fatalf("AddPage: %v", err)
	}
	if err := addPageNumber(cr, page, 44, 1, 1, "Приложение А"); err != nil {
		t.Fatalf("addPageNumber: %v", err)
	}

	// Сразу после отрисовки шрифта в ресурсах страницы ещё нет: creator
	// материализует блоки и собирает шрифты только при записи. Поэтому ремонт
	// вешаем в хук доступа к PdfWriter — он вызывается после Finalize, но до
	// сериализации страниц.
	repair := fontutil.NewCidWidthRepair()
	repaired := 0
	cr.SetPdfWriterAccessFunc(func(_ *pdf.PdfWriter) error {
		var err error
		repaired, err = repair.RepairPage(page)
		return err
	})
	if err := cr.WriteToFile(out); err != nil {
		t.Fatalf("WriteToFile: %v", err)
	}
	if repaired == 0 {
		t.Fatal("ремонт в хуке не нашёл ни одного составного шрифта: тест не проверяет то, что думает")
	}

	f, err := os.Open(out)
	if err != nil {
		t.Fatalf("открыть %s: %v", out, err)
	}
	defer f.Close()
	r, err := pdf.NewPdfReader(f)
	if err != nil {
		t.Fatalf("NewPdfReader: %v", err)
	}
	read, err := r.GetPage(1)
	if err != nil {
		t.Fatalf("GetPage(1): %v", err)
	}
	broken, err := fontutil.NewCidWidthRepair().CountBrokenOnPage(read)
	if err != nil {
		t.Fatalf("CountBrokenOnPage: %v", err)
	}
	if broken != 0 {
		t.Errorf("в готовом PDF %d расхождений /W с hmtx — Acrobat покажет наезд букв", broken)
	}
}

// TestNumberFontWidthsBrokenWithoutRepair фиксирует, что тест выше не проходит
// «просто так»: без ремонта unipdf действительно оставляет битый /W.
func TestNumberFontWidthsBrokenWithoutRepair(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "unrepaired.pdf")

	cr := c.New()
	page := pdf.NewPdfPage()
	page.MediaBox = &pdf.PdfRectangle{Urx: a4LandscapeW, Ury: a4LandscapeH}
	if err := cr.AddPage(page); err != nil {
		t.Fatalf("AddPage: %v", err)
	}
	if err := addPageNumber(cr, page, 44, 1, 1, "Приложение А"); err != nil {
		t.Fatalf("addPageNumber: %v", err)
	}
	// Ремонт намеренно НЕ вызываем.
	if err := cr.WriteToFile(out); err != nil {
		t.Fatalf("WriteToFile: %v", err)
	}

	f, err := os.Open(out)
	if err != nil {
		t.Fatalf("открыть %s: %v", out, err)
	}
	defer f.Close()
	r, err := pdf.NewPdfReader(f)
	if err != nil {
		t.Fatalf("NewPdfReader: %v", err)
	}
	read, err := r.GetPage(1)
	if err != nil {
		t.Fatalf("GetPage(1): %v", err)
	}
	broken, err := fontutil.NewCidWidthRepair().CountBrokenOnPage(read)
	if err != nil {
		t.Fatalf("CountBrokenOnPage: %v", err)
	}
	if broken == 0 {
		t.Skip("unipdf больше не воспроизводит баг makeCIDWidthArr — регрессионный тест можно упростить")
	}
	t.Logf("подтверждено: без ремонта расхождений %d", broken)
}
