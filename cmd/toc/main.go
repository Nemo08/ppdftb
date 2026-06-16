package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"

	"log/slog"

	"github.com/Nemo08/ppdftb/pkg/slogutil"
	"github.com/Nemo08/ppdftb/pkg/toc"
)

var version string

func main() {
	flag.Parse()
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("toc", flag.ContinueOnError)
	var tf, td, pd, Level string
	var tn int
	var Version, appendix bool

	fs.StringVar(&tf, "tf", "", "файл шаблона содержания (*.docx)")
	fs.StringVar(&td, "td", "", "папка для собранного содержания")
	fs.StringVar(&pd, "pd", "", "папка с PDF файлами")
	fs.IntVar(&tn, "tn", 3, "номер страницы содержания в собранном файле")
	fs.StringVar(&Level, "l", "error", "debug, info, warn, error")
	fs.BoolVar(&Version, "v", false, "версия программы")

	// Сканируем -appendix вручную, потому что Go flag.Parse()
	// останавливается на первом позиционном аргументе.
	filtered := make([]string, 0, len(args))
	for _, a := range args {
		if a == "-appendix" || a == "--appendix" {
			appendix = true
		} else {
			filtered = append(filtered, a)
		}
	}
	if err := fs.Parse(filtered); err != nil {
		return 1
	}
	ctx := context.Background()

	slogutil.Setup(Level)

	if Version {
		fmt.Println(version)
		return 0
	}

	var src, pdfDir, out string
	page := tn

	if tf != "" {
		src = tf
		out = td
		pdfDir = pd
	} else {
		posArgs := fs.Args()
		if len(posArgs) < 3 {
			slog.ErrorContext(ctx, "Обязательные аргументы: source-file output-folder pdf-folder [page]")
			return 1
		}
		src = posArgs[0]
		out = posArgs[1]
		pdfDir = posArgs[2]
		if len(posArgs) > 3 {
			var err error
			page, err = strconv.Atoi(posArgs[3])
			if err != nil {
				slog.ErrorContext(ctx, "page должен быть числом")
				return 1
			}
		}
	}

	if src == "" || out == "" || pdfDir == "" {
		slog.ErrorContext(ctx, "Обязательные аргументы: source, output, pdf")
		return 1
	}

	var tocOpts []toc.Option
	if appendix {
		tocOpts = append(tocOpts, toc.WithAppendix())
	}
	if err := toc.Make(ctx, src, pdfDir, out, page, tocOpts...); err != nil {
		slog.ErrorContext(ctx, "ошибка генерации содержания", slog.String("err", err.Error()))
		return 1
	}
	return 0
}
