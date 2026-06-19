//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"log/slog"

	"github.com/Nemo08/ppdftb/pkg/slogutil"
)

var version string

func main() {
	var (
		cfg       Config
		logLevel  string
		showVer   bool
	)

	flag.StringVar(&cfg.TplDir, "t", "Шаблон тома", "папка с DOCX-шаблонами")
	flag.StringVar(&cfg.TplsDir, "s", "Шаблоны", "папка с общими шаблонами (toc)")
	flag.StringVar(&cfg.RootDir, "x", ".", "корневая папка с XML-данными")
	flag.StringVar(&cfg.OutFile, "o", "", "выходной PDF (пусто = авто из XML)")
	flag.StringVar(&cfg.PDFDir, "p", "", "временная папка PDF (по умолч. {root}/PDF)")
	flag.StringVar(&cfg.DocsDir, "d", "", "временная папка DOCX (по умолч. {root}/Документы тома)")
	flag.StringVar(&cfg.PicsDir, "pp", "", "папка с картинками для подстановки в шаблон (по умолч. {root}/pics)")
	flag.IntVar(&cfg.WordPool, "w", 4, "размер пула Word")
	flag.IntVar(&cfg.TocPageFrom, "tn", 4, "номер страницы оглавления в итоговом PDF")
	flag.IntVar(&cfg.PageFrom, "pf", 3, "номер первой страницы для нумерации")
	flag.IntVar(&cfg.NumberFrom, "nf", 3, "начальный номер для нумерации")
	flag.StringVar(&logLevel, "l", "error", "debug, info, warn, error")
	flag.BoolVar(&showVer, "v", false, "версия программы")

	flag.Parse()

	if showVer {
		fmt.Println("ppdftb engine2")
		if version != "" {
			fmt.Println(version)
		}
		return
	}

	slogutil.Setup(logLevel)

	if err := Run(context.Background(), &cfg); err != nil {
		slog.Error("pipeline failed", slog.String("err", err.Error()))
		os.Exit(1)
	}
}
