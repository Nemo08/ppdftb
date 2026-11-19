package main

import (
	"fmt"
	"os"

	"github.com/oliverpool/unipdf/v3/core"
	"github.com/oliverpool/unipdf/v3/model"
)

func main() {
	f, err := os.Open("E:/Yandex.Disk.Мой/Yandex.Disk/Dev/GO/DOC/ppdftb/tests4/Шаблон тома/2. Обложка и титульный лист.pdf")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	rdr, err := model.NewPdfReader(f)
	if err != nil {
		panic(err)
	}
	page, _ := rdr.GetPage(1)
	res := page.Resources
	names := []string{}
	if fdict, ok := core.ResolveReference(res.Font).(*core.PdfObjectDictionary); ok {
		for _, k := range fdict.Keys() {
			names = append(names, string(k))
		}
	}
	texts := []string{"Министерство", "2026", "02/01-25", "СРО-И-013-25122009"}
	for _, n := range names {
		obj, ok := res.GetFontByName(core.PdfObjectName(n))
		if !ok {
			continue
		}
		pf, err := model.NewPdfFontFromPdfObject(obj)
		if err != nil {
			fmt.Println(n, "ERR", err)
			continue
		}
		d, ok := core.GetDict(pf.ToPdfObject())
		var base, sub string
		if ok {
			if nn, ok2 := d.Get("BaseFont").(*core.PdfObjectName); ok2 {
				base = string(*nn)
			}
			if s, ok2 := d.Get("Subtype").(*core.PdfObjectName); ok2 {
				sub = string(*s)
			}
		}
		fmt.Printf("=== /%s BaseFont=%s Subtype=%s\n", n, base, sub)
		for _, t := range texts {
			b, miss := pf.RunesToCharcodeBytes([]rune(t))
			hexs := ""
			for _, c := range b {
				hexs += fmt.Sprintf(" %02X", c)
			}
			fmt.Printf("  %-18q miss=%d bytes=%s\n", t, miss, hexs)
		}
	}
}
