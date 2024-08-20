package main

import (
	"context"
	"flag"
	"os"

	"golang.org/x/exp/slog"

	"github.com/Nemo08/ppdftb/pkg/pdf"
)

func main() {
	//Установка логгера
	opts := &slog.HandlerOptions{
		Level:     slog.LevelError,
		AddSource: true,
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, opts))

	slog.SetDefault(logger)
	ctx := context.Background()

	//Получение флагов
	inputFileName := flag.String("if", "", "файл для нумерации")
	outputFileName := flag.String("of", "", "выходной файл")
	pageFrom := flag.Int("pf", 1, "c какой физической страницы нумеровать")
	numberFrom := flag.Int("nf", 1, "c какого номера начинать")

	flag.Parse()

	//Проверка флагов
	if (*inputFileName + *outputFileName) == "" {
		slog.ErrorCtx(ctx, "Во входных параметрах должен быть входной файл и выходной файл")
		os.Exit(1)
	}

	err := pdf.MakePagination(ctx, *inputFileName, *outputFileName, *pageFrom, *numberFrom)
	if err != nil {
		slog.ErrorCtx(ctx, "Ошибка добавления нумерации", err)
		os.Exit(1)
	}
}
