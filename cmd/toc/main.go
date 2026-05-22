// toc — Table of Contents. Формирует файл оглавления в формате DOCX на основе
// шаблона и набора PDF-файлов с нумерацией страниц.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"log/slog"

	"github.com/Nemo08/ppdftb/pkg/slogutil"
	"github.com/Nemo08/ppdftb/pkg/toc"
)

var version string

var CLI struct {
	Src string `arg:"" name:"source file" short:"s" help:"файл шаблона содержания, подготовленный в формате *.docx" type:"existingfile" optional:""`
	Out string `arg:"" name:"output file" short:"o" help:"папка для собранного из шаблона содержания" type:"existingdir" optional:""`
	Pdf string `arg:"" name:"pdf folder" short:"p" help:"папка *.pdf файлов для которых строится содержание" type:"existingdir" optional:""`

	Page    int    `arg:"" name:"page" short:"n" help:"номер страницы содержания в собранном файле" default:"3"`
	Level   string `name:"log" short:"l" help:"debug,info,warn,error" enum:"debug,info,warn,error" default:"error"`
	Version bool   `name:"version" short:"v" help:"версия программы"`
}

func main() {
	_ = kong.Parse(
		&CLI,
		kong.Description("Утилита на основе шаблона *.docx строит файл содержания в формате *.docx по папке с pdf файлами"),
	)
	ctx := context.Background()

	slogutil.Setup(CLI.Level)

	if CLI.Version {
		fmt.Println(version)
		return
	}
	if CLI.Src == "" || CLI.Out == "" || CLI.Pdf == "" {
		slog.ErrorContext(ctx, "Обязательные аргументы: source file, output file, pdf folder")
		os.Exit(1)
	}

	toc.Make(ctx, CLI.Src, CLI.Pdf, CLI.Out, CLI.Page)
}
