// Command pnpdf склеивает страницы PDF в один файл.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"log/slog"

	"github.com/Nemo08/ppdftb/pkg/pdf"
	"github.com/Nemo08/ppdftb/pkg/slogutil"
)

var version string

func main() {
	flag.Parse()
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("pnpdf", flag.ContinueOnError)
	var InputFile, OutputFile, Level string
	var PageFrom, NumberFrom int
	var Version, Appendix bool

	fs.StringVar(&InputFile, "if", "", "входной PDF файл для нумерации")
	fs.StringVar(&OutputFile, "of", "", "выходной PDF файл")
	fs.IntVar(&PageFrom, "pf", 1, "с какой физической страницы нумеровать")
	fs.IntVar(&NumberFrom, "nf", 1, "с какого номера начинать")
	fs.StringVar(&Level, "l", "error", "debug, info, warn, error")
	fs.BoolVar(&Version, "v", false, "версия программы")
	fs.BoolVar(&Appendix, "appendix", false,
		"читать outline PDF и добавлять \"Прил. А\" перед номером на страницах приложений")

	if err := fs.Parse(args); err != nil {
		return 1
	}
	ctx := context.Background()
	slogutil.Setup(Level)

	if Version {
		fmt.Println(version)
		return 0
	}
	if InputFile == "" {
		slog.ErrorContext(ctx, "Должен быть указан входной файл (-if)")
		return 1
	}
	if OutputFile == "" {
		slog.ErrorContext(ctx, "Должен быть указан выходной файл (-of)")
		return 1
	}

	var opts []pdf.PaginationOption
	if Appendix {
		opts = append(opts, pdf.WithPaginationAppendix())
	}

	if err := pdf.MakePagination(ctx, InputFile, OutputFile, PageFrom, NumberFrom, opts...); err != nil {
		slog.ErrorContext(ctx, "Ошибка добавления нумерации", slog.String("err", err.Error()))
		return 1
	}
	return 0
}
