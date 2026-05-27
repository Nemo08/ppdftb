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
	var InputFile, OutputFile, Level string
	var PageFrom, NumberFrom int
	var Version, Appendix bool

	flag.StringVar(&InputFile, "if", "", "входной PDF файл для нумерации")
	flag.StringVar(&OutputFile, "of", "", "выходной PDF файл")
	flag.IntVar(&PageFrom, "pf", 1, "с какой физической страницы нумеровать")
	flag.IntVar(&NumberFrom, "nf", 1, "с какого номера начинать")
	flag.StringVar(&Level, "l", "error", "debug, info, warn, error")
	flag.BoolVar(&Version, "v", false, "версия программы")
	flag.BoolVar(&Appendix, "appendix", false,
		"читать outline PDF и добавлять \"Прил. А\" перед номером на страницах приложений")

	flag.Parse()
	ctx := context.Background()
	slogutil.Setup(Level)

	if Version {
		fmt.Println(version)
		return
	}
	if InputFile == "" {
		slog.ErrorContext(ctx, "Должен быть указан входной файл (-if)")
		os.Exit(1)
	}
	if OutputFile == "" {
		slog.ErrorContext(ctx, "Должен быть указан выходной файл (-of)")
		os.Exit(1)
	}

	var opts []pdf.PaginationOption
	if Appendix {
		opts = append(opts, pdf.WithPaginationAppendix())
	}

	if err := pdf.MakePagination(ctx, InputFile, OutputFile, PageFrom, NumberFrom, opts...); err != nil {
		slog.ErrorContext(ctx, "Ошибка добавления нумерации", slog.String("err", err.Error()))
		os.Exit(1)
	}
}
