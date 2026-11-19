package main

import (
	"fmt"
	"os"
	"strings"

	core "github.com/oliverpool/unipdf/v3/core"
	model "github.com/oliverpool/unipdf/v3/model"

	"github.com/Nemo08/ppdftb/pkg/fontutil"
)

func main() {
	path := os.Args[1]
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	r, err := model.NewPdfReader(f)
	if err != nil {
		panic(err)
	}
	page, err := r.GetPage(1)
	if err != nil {
		panic(err)
	}

	vals := []string{
		"Министерство градостроительства и архитектуры Тамбовской области",
		"Инженерно-геодезические изыскания",
		"2026",
		"02/01-25",
		"ООО «СМУ-7 Тамбов»",
	}

	names := []string{}
	if page.Resources != nil && page.Resources.Font != nil {
		if dict, ok := core.GetDict(page.Resources.Font); ok {
			for _, n := range dict.Keys() {
				names = append(names, string(n))
			}
		}
	}
	for _, n := range names {
		fo, ok := page.Resources.GetFontByName(core.PdfObjectName(n))
		if !ok {
			fmt.Printf("== /%s: нет ресурса\n", n)
			continue
		}
		fn, err := model.NewPdfFontFromPdfObject(fo)
		if err != nil {
			fmt.Printf("== /%s: parse err %v\n", n, err)
			continue
		}
		cov := fontutil.FontCmap(fn)
		base := fontutil.BaseFontName(fn)
		composite := fontutil.IsCompositeFont(fn)
		subset := strings.Contains(base, "+")
		fmt.Printf("== /%s base=%q comp=%v subset=%v cmap=%d\n", n, base, composite, subset, len(cov))
		for _, v := range vals {
			missing := []rune{}
			seen := map[rune]bool{}
			for _, rn := range v {
				if seen[rn] {
					continue
				}
				seen[rn] = true
				if cov != nil && !cov[rn] {
					missing = append(missing, rn)
				}
			}
			covered := fontutil.CoveredBy(fn, v)
			missStr := ""
			if len(missing) > 0 {
				missStr = " missing=" + strings.ReplaceAll(string(missing), " ", "·")
			}
			_ = composite
			_ = subset
			fmt.Printf("   %-55q covered=%v%s\n", v, covered, missStr)
			if composite && len(missing) > 0 {
				// Показываем, что unipdf «кодирует» недостающие руны в номера
				// CID (0x30..0x39 для цифр, 0x04XX для кириллицы), а не в глифы:
				// glyf[0x041C] в 64-глифовом сабсете пуст → .notdef при рендере.
				b, _ := fn.RunesToCharcodeBytes([]rune(v))
				fmt.Printf("      unipdf bytes=%X\n", b)
			}
		}
	}
}
