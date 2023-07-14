package main

import (
	"context"
	"flag"
	"os"

	"golang.org/x/exp/slog"

	"mpdf/pkg/pdf"
)

type Bookmark struct {
	num  uint64
	text string
}

func main() {
	//Установка логгера
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, opts))
	slog.SetDefault(logger)
	ctx := context.Background()

	//Получение флагов
	outFileName := flag.String("o", "out.pdf", "output pdf file")
	inDirName := flag.String("d", "", "input directory")

	flag.Parse()

	//Проверка флагов
	if *inDirName == "" {
		slog.ErrorCtx(ctx, "Directory flag is empty")
		os.Exit(1)
	}

	//Проверка наличия исходной папки
	if _, err := os.Stat(*inDirName); os.IsNotExist(err) {
		slog.ErrorCtx(ctx, "Directory does not exists", slog.String("folder", *inDirName))
		os.Exit(1)
	}

	err := pdf.Merge(ctx, *inDirName, *outFileName)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		os.Exit(1)
	}
}
