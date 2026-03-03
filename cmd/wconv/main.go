package main

import (
	"context"
	"fmt"
	"os"
	"time"

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
	Src     []string `name:"source" short:"s" help:"файл или папка для конвертации" type:"*os.File"`
	Out     string   `name:"output" short:"o" help:"папка для сконвертированных *.pdf файлов" type:"existingdir" required:""`
	Outd    string   `name:"outputd" short:"d" help:"папка для собранных *.docx файлов" type:"existingdir" optional:""`
	Level   string   `name:"log" short:"l" help:"уровни логгирования: debug,info,warn,error" enum:"debug,info,warn,error" default:"error"`
	Version bool     `name:"version" short:"v" help:"версия программы"`

	Dx []string `name:"xml" short:"i" help:"данные для шаблона" type:"*os.File" group:"Files" optional:"" xor:"Files,Folders"`

	DxF string `name:"xmlf" short:"x" help:"корневая папка с файлами *.xml данных для шаблона" type:"*os.File" group:"Folders" optional:"" xor:"Files,Folders"`
	DxL int    `name:"up" short:"u" help:"на сколько папок выше смотреть" optional:""`
}

var (
	version string = "0.3j 2603"
)

func main() {
	_ = kong.Parse(&CLI)

	if CLI.Version {
		fmt.Println("version:", version)
	}

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
		tempDir, err = os.MkdirTemp(os.TempDir(), "wconv")
		defer os.RemoveAll(tempDir)
		if err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
	} else {
		tempDir = CLI.Outd
	}
	ctx := context.Background()

	var data [][]byte

	if len(CLI.Dx) != 0 {
		data, err = conv.GetDataContent(ctx, CLI.Dx)
		if err != nil {
			slog.ErrorCtx(ctx, "Ошибка получения данных шаблона/шаблонов", err)
			os.Exit(1)
		}
	} else {
		if CLI.DxF != "" {
			fmt.Println(CLI.DxF, CLI.DxL)
			data, err = conv.FindXMLFiles(CLI.DxF, CLI.DxL)
			if err != nil {
				slog.ErrorCtx(ctx, "Ошибка получения данных шаблона/шаблонов", err)
				os.Exit(1)
			}
		}
	}

	mergedData, err := conv.DataMerge(data)
	//fmt.Println(string(mergedData), err)
	if err != nil {
		slog.ErrorCtx(ctx, "Ошибка данных", err)
		os.Exit(1)
	}

	err = conv.TplToDocxJJack2(ctx, CLI.Src, tempDir, mergedData)

	if err != nil {
		slog.ErrorCtx(ctx, "Ошибка шаблонов", err)
		os.Exit(1)
	}
	start := time.Now()
	err = conv.FilesToPdf(ctx, []string{tempDir}, CLI.Out)

	/*
		converter := conv.NewGotenbergConverter("http://192.168.1.110:3000")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		err = converter.GotenbergFilesToPdf(ctx, []string{tempDir}, CLI.Out)
	*/
	fmt.Println("операция:", time.Since(start))

	if err != nil {
		slog.ErrorCtx(ctx, "Ошибка конвертации", err)
		os.Exit(1)
	}

}
