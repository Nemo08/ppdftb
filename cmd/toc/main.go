package main

import (
	"context"
	"flag"
	"os"

	"golang.org/x/exp/slog"

	"github.com/Nemo08/ppdftb/pkg/toc"
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
	templateFileName := flag.String("tf", "", "input file doc/docx template of TOC")
	templatePageNumber := flag.Int("tn", 3, "number of template's first page in united file")

	pdfDirectoryName := flag.String("pd", "", "pdf directory")
	compiledTemplateDirectoryName := flag.String("td", "", "directory, where compiled template saved")

	flag.Parse()

	//Проверка заполнения параметров
	if (*templateFileName == "") || (*pdfDirectoryName == "") || (*compiledTemplateDirectoryName == "") {
		slog.Error("Все параметры командной строки обязательны. Один или несколько из них не заданы")
		os.Exit(1)
	}

	toc.Make(ctx, *templateFileName, *pdfDirectoryName, *compiledTemplateDirectoryName, *templatePageNumber)
}
