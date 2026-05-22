// pnpdf — Page Number PDF. Добавляет нумерацию страниц в готовый PDF-файл
// с возможностью указать начальную физическую страницу и начальный номер.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"log/slog"

	"github.com/Nemo08/ppdftb/pkg/pdf"
	"github.com/Nemo08/ppdftb/pkg/slogutil"
)

var version string

var CLI struct {
	InputFile  string `name:"if" short:"i" help:"входной PDF файл для нумерации" type:"existingfile" optional:""`
	OutputFile string `name:"of" short:"o" help:"выходной PDF файл" optional:""`
	PageFrom   int    `name:"pf" short:"p" help:"с какой физической страницы нумеровать" default:"1"`
	NumberFrom int    `name:"nf" short:"n" help:"с какого номера начинать" default:"1"`
	Level      string `name:"log" short:"l" help:"debug,info,warn,error" enum:"debug,info,warn,error" default:"error"`
	Version    bool   `name:"version" short:"v" help:"версия программы"`
}

func main() {
	_ = kong.Parse(&CLI)
	ctx := context.Background()

	slogutil.Setup(CLI.Level)

	if CLI.Version {
		fmt.Println(version)
		return
	}

	if CLI.InputFile == "" {
		slog.ErrorContext(ctx, "Должен быть указан входной файл (--if)")
		os.Exit(1)
	}
	if CLI.OutputFile == "" {
		slog.ErrorContext(ctx, "Должен быть указан выходной файл (--of)")
		os.Exit(1)
	}

	err := pdf.MakePagination(ctx, CLI.InputFile, CLI.OutputFile, CLI.PageFrom, CLI.NumberFrom)
	if err != nil {
		slog.ErrorContext(ctx, "Ошибка добавления нумерации", slog.String("err", err.Error()))
		os.Exit(1)
	}
}
