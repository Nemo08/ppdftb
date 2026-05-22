// aconv — AutoCAD converter. Конвертирует чертежи DWG/DXF в PDF через
// установленный AutoCAD (COM-автоматизация). Работает только на Windows.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"log/slog"

	conv "github.com/Nemo08/ppdftb/pkg/convert"
	"github.com/Nemo08/ppdftb/pkg/slogutil"
)

var version string

var CLI struct {
	InputFile  string `name:"if" short:"i" help:"файл DWG/DXF для конвертации" type:"existingfile" optional:""`
	InputDir   string `name:"id" short:"d" help:"папка с DWG/DXF файлами" type:"existingdir" optional:""`
	OutputDir  string `name:"od" short:"o" help:"папка для сконвертированных PDF файлов" type:"existingdir" optional:""`
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
	if CLI.OutputDir == "" {
		slog.ErrorContext(ctx, "Должна быть указана папка для PDF (--od)")
		os.Exit(1)
	}
	if CLI.InputFile == "" && CLI.InputDir == "" {
		slog.ErrorContext(ctx, "Должен быть указан входной файл (--if) или папка (--id)")
		os.Exit(1)
	}

	err := conv.A2pdf(ctx, CLI.InputFile, CLI.InputDir, CLI.OutputDir)
	if err != nil {
		slog.ErrorContext(ctx, "Ошибка конвертации", slog.String("err", err.Error()))
		os.Exit(1)
	}
}
