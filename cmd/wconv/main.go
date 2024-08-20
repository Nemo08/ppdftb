package main

import (
	"context"
	"os"

	"golang.org/x/exp/slog"

	conv "github.com/Nemo08/ppdftb/pkg/convert"
	"github.com/alecthomas/kong"
)

var logLevels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
	"none":  -8,
}

var CLI struct {
	Src   []string `name:"source" short:"s" help:"файл или папка для конвертации" type:"*os.File"`
	Out   string   `name:"output" short:"o" help:"папка для конвертированных файлов" type:"existingdir" required:""`
	Src   []string `name:"source" short:"s" help:"файл или папка для конвертации" type:"*os.File"`
	Level string   `name:"log" short:"l" help:"debug,info,warn,error" enum:"debug,info,warn,error" default:"error"`
}

func main() {
	_ = kong.Parse(&CLI)

	if CLI.Level != "error" {
		//Установка логгера
		opts := &slog.HandlerOptions{
			Level:     logLevels[CLI.Level],
			AddSource: true,
		}

		logger := slog.New(slog.NewTextHandler(os.Stdout, opts))
		slog.SetDefault(logger)
	}

	ctx := context.Background()

	err := conv.FilesToPdf(ctx, CLI.Src, CLI.Out)
	if err != nil {
		slog.ErrorCtx(ctx, "Ошибка конвертации", err)
		os.Exit(1)
	}
}
