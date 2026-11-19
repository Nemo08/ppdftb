package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nemo08/ppdftb/pkg/pdfpl"
)

// demo_out/main.go — регенерация одной обложки тома новым движком pdfpl.
// -src путь к PDF с плейсхолдерами, -dst итоговый файл, -xml папка с XML
// (значения мержатся, первые выигрывают), иначе используются значения по умолчанию.
func main() {
	src := flag.String("src", "", "исходный PDF с плейсхолдерами {{.Key}}")
	dst := flag.String("dst", "", "итоговый PDF")
	xmlDir := flag.String("xml", "", "папка с *.xml значениями (мерж, первые выигрывают)")
	flag.Parse()

	if *src == "" || *dst == "" {
		fmt.Fprintln(os.Stderr, "нужны -src и -dst")
		os.Exit(2)
	}

	values := map[string]string{
		"ESYear":               "2026",
		"ESNumber":             "1",
		"ESVolNumber":          "2",
		"ESName":               "Том 1",
		"ESType":               "Основной",
		"OrgName":              "ООО «СМУ-7 Тамбов»",
		"License":              "Лицензия № 11-042 от 13.03.2026",
		"ObjectName":           "Инженерно-геодезические изыскания",
		"DeveloperOrgFullName": "Министерство градостроительства и архитектуры Тамбовской области",
		"OrgHeadFIO":           "Иванов Пётр Петрович",
	}
	if *xmlDir != "" {
		files, err := filepath.Glob(filepath.Join(*xmlDir, "*.xml"))
		if err != nil || len(files) == 0 {
			fmt.Fprintf(os.Stderr, "XML не найдены: %v\n", err)
			os.Exit(2)
		}
		merged := map[string]string{}
		for _, f := range files {
			fh, err := os.Open(f)
			if err != nil {
				fmt.Fprintf(os.Stderr, "open %s: %v\n", f, err)
				os.Exit(2)
			}
			v, err := pdfpl.ValuesFromXML(fh)
			_ = fh.Close()
			if err != nil {
				fmt.Fprintf(os.Stderr, "ValuesFromXML(%s): %v\n", f, err)
				os.Exit(2)
			}
			for k, val := range v {
				if _, ok := merged[k]; !ok {
					merged[k] = val
				}
			}
		}
		for k, val := range merged {
			values[k] = val
		}
	}

	res, err := pdfpl.ReplaceFile(context.Background(), *src, *dst, values)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ReplaceFile:", err)
		os.Exit(1)
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	fmt.Printf("Pages=%d Streams=%d Replaced=%d Skipped=%d Values=%d\n",
		res.Pages, res.Streams, res.Replaced, res.Skipped, len(keys))
	fmt.Println("Keys:", strings.Join(keys, ", "))
}
