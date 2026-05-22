// mpdf — Merge PDF. Объединяет несколько PDF-файлов из папки в один,
// сохраняя закладки (outline) из исходных файлов.
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
	Dir     string `name:"dir" short:"d" help:"папка с PDF файлами для объединения" type:"existingdir" optional:""`
	Out     string `name:"out" short:"o" help:"выходной PDF файл" default:"out.pdf"`
	Level   string `name:"log" short:"l" help:"debug,info,warn,error" enum:"debug,info,warn,error" default:"error"`
	Version bool   `name:"version" short:"v" help:"версия программы"`
}

func main() {
	_ = kong.Parse(&CLI)
	ctx := context.Background()

	slogutil.Setup(CLI.Level)

	if CLI.Version {
		fmt.Println(version)
		return
	}
	if CLI.Dir == "" {
		slog.ErrorContext(ctx, "Должна быть указана папка с PDF (--dir)")
		os.Exit(1)
	}

	err := pdf.Merge(ctx, CLI.Dir, CLI.Out)
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		os.Exit(1)
	}
}
