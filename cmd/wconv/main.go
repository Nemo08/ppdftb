package main

import (
	"context"
	"io/ioutil"
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
	Out   string   `name:"output" short:"o" help:"папка для конвертированных *.pdf файлов" type:"existingdir" required:""`
	Outd  string   `name:"outputd" short:"d" help:"папка для собранных *.docx файлов" type:"existingdir" group:"tpl" optional:""`
	Dx    []string `name:"xml data source" short:"i" help:"данные для шаблона" type:"*os.File" group:"tpl" optional:""`
	Level string   `name:"log" short:"l" help:"debug,info,warn,error" enum:"debug,info,warn,error" default:"error"`
}

var (
	version string = "0.1"
)

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

	var tempDir string = ""
	var err error

	if CLI.Outd == "" {
		tempDir, err = ioutil.TempDir(os.TempDir(), "wconv")
		if err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
		defer os.RemoveAll(tempDir)
	} else {
		tempDir = CLI.Outd
	}
	ctx := context.Background()

	var data map[string]string

	if len(CLI.Dx) != 0 {
		data, err = conv.GetData(ctx, []string{"data.xml"})
		if err != nil {
			slog.ErrorCtx(ctx, "Ошибка получения данных шаблона", err)
			os.Exit(1)
		}
	}

	err = conv.TplToDocx(ctx, CLI.Src, tempDir, data)
	if err != nil {
		slog.ErrorCtx(ctx, "Ошибка шаблонов", err)
		os.Exit(1)
	}
	err = conv.FilesToPdf(ctx, []string{tempDir}, CLI.Out)

	if err != nil {
		slog.ErrorCtx(ctx, "Ошибка конвертации", err)
		os.Exit(1)
	}
}
