package main

import (
	"context"
	"os"

	"github.com/alecthomas/kong"
	"golang.org/x/exp/slog"

	"github.com/Nemo08/ppdftb/pkg/toc"
)

var logLevels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
	"none":  -8,
}

var CLI struct {
	Src string `arg:"" name:"source file" short:"s" help:"файл шаблона содержания, подготовленный в формате *.docx" type:"existingfile" `
	Out string `arg:"" name:"output file" short:"o" help:"папка для собранного из шаблона содержания" type:"existingdir"`
	Pdf string `arg:"" name:"pdf folder" short:"p" help:"папка *.pdf файлов для которых строится содержание" type:"existingdir"`

	Page  int    `arg:"" name:"page" short:"p" help:"номер страницы содержания в собранном файле" default:3`
	Level string `name:"log" short:"l" help:"debug,info,warn,error" enum:"debug,info,warn,error" default:"error"`
}

func main() {
	_ = kong.Parse(
		&CLI,
		kong.Description("Утилита на основе шаблона *.docx строит файл содержания в формате *.docx по папке с pdf файлами"),
	)
	ctx := context.Background()

	if CLI.Level != "error" {
		//Установка логгера
		opts := &slog.HandlerOptions{
			Level:     logLevels[CLI.Level],
			AddSource: true,
		}

		logger := slog.New(slog.NewTextHandler(os.Stdout, opts))
		slog.SetDefault(logger)
	}

	toc.Make(ctx, CLI.Src, CLI.Pdf, CLI.Out, CLI.Page)
}
