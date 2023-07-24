package main

import (
	"context"
	"flag"
	"os"

	"golang.org/x/exp/slog"

	conv "github.com/Nemo08/ppdftb/pkg/convert"
)

func main() {
	//Установка логгера
	opts := &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: true,
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, opts))
	slog.SetDefault(logger)
	ctx := context.Background()

	//Получение флагов

	inputFileName := flag.String("if", "", "файл для конвертации")
	inputDirName := flag.String("id", "", "папка, откуда брать файлы")
	outputDirName := flag.String("od", "", "папка, куда сложить конвертированный файл")
	_ = outputDirName

	flag.Parse()

	//Проверка флагов
	if (*inputFileName + *inputDirName) == "" {
		slog.ErrorCtx(ctx, "Во входных параметрах должен быть указан либо входной файл, либо папка")
	}
	err := conv.A2pdf(ctx, *inputFileName, *outputDirName)

	if err != nil {
		slog.ErrorCtx(ctx, "Ошибка конвертации", err)
		os.Exit(1)
	}
}
