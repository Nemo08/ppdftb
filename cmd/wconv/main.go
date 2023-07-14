package main

import (
	"context"
	"flag"
	"log"
	"os"

	"golang.org/x/exp/slog"

	conv "github.com/Nemo08/ppdftb/pkg/convert"
)

func main() {
	//Установка логгера
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, opts))
	slog.SetDefault(logger)
	ctx := context.Background()

	//Получение флагов

	inputFileName := flag.String("if", "", "файл для конвертации")
	inputDirName := flag.String("id", "", "папка, откуда брать файлы")
	outputDirName := flag.String("od", "", "папка, куда сложить конвертированный файл")

	flag.Parse()

	//Проверка флагов
	if (*inputFileName + *inputDirName) == "" {
		log.Fatal("Во входных параметрах должен быть указан либо входной файл, либо папка")
	}

	err := conv.FilesToPdf(ctx, *inputFileName, *inputDirName, *outputDirName)
	if err != nil {
		slog.ErrorCtx(ctx, "Ошибка конвертации", err)
		os.Exit(1)
	}

}
