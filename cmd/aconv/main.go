package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"log/slog"

	conv "github.com/Nemo08/ppdftb/pkg/convert"
	"github.com/Nemo08/ppdftb/pkg/slogutil"
)

var version string

func main() {
	var InputFile, InputDir, OutputDir, Level string
	var Version bool

	flag.StringVar(&InputFile, "if", "", "файл DWG/DXF для конвертации")
	flag.StringVar(&InputDir, "id", "", "папка с DWG/DXF файлами")
	flag.StringVar(&OutputDir, "od", "", "папка для сконвертированных PDF файлов")
	flag.StringVar(&Level, "log", "error", "debug, info, warn, error")
	flag.BoolVar(&Version, "v", false, "версия программы")

	flag.Parse()
	ctx := context.Background()

	slogutil.Setup(Level)

	if Version {
		fmt.Println(version)
		return
	}
	if OutputDir == "" {
		slog.ErrorContext(ctx, "Должна быть указана папка для PDF (-od)")
		os.Exit(1)
	}
	if InputFile == "" && InputDir == "" {
		slog.ErrorContext(ctx, "Должен быть указан входной файл (-if) или папка (-id)")
		os.Exit(1)
	}

	err := conv.A2pdf(ctx, InputFile, InputDir, OutputDir)
	if err != nil {
		slog.ErrorContext(ctx, "Ошибка конвертации", slog.String("err", err.Error()))
		os.Exit(1)
	}
}
