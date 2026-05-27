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
	var tf, td, pd, Level string
	var tn int
	var Version, appendix bool

	flag.StringVar(&tf, "tf", "", "файл шаблона содержания (*.docx)")
	flag.StringVar(&td, "td", "", "папка для собранного содержания")
	flag.StringVar(&pd, "pd", "", "папка с PDF файлами")
	flag.IntVar(&tn, "tn", 3, "номер страницы содержания в собранном файле")
	flag.StringVar(&Level, "l", "error", "debug, info, warn, error")
	flag.BoolVar(&Version, "v", false, "версия программы")
	flag.BoolVar(&appendix, "appendix", false, "режим приложений (автодетект маркеров-разделителей)")

	flag.Parse()
	ctx := context.Background()

	slogutil.Setup(Level)

	if Version {
		fmt.Println(version)
		return
	}

	var src, pdfDir, out string
	page := tn

	if tf != "" {
		src = tf
		out = td
		pdfDir = pd
	} else {
		args := flag.Args()
		if len(args) < 3 {
			slog.ErrorContext(ctx, "Обязательные аргументы: source-file output-folder pdf-folder [page]")
			os.Exit(1)
		}
		src = args[0]
		out = args[1]
		pdfDir = args[2]
		if len(args) > 3 {
			var err error
			page, err = strconv.Atoi(args[3])
			if err != nil {
				slog.ErrorContext(ctx, "page должен быть числом")
				os.Exit(1)
			}
		}
	}

	if src == "" || out == "" || pdfDir == "" {
		slog.ErrorContext(ctx, "Обязательные аргументы: source, output, pdf")
		os.Exit(1)
	}

	var tocOpts []toc.Option
	if appendix {
		tocOpts = append(tocOpts, toc.WithAppendix())
	}
	if err := toc.Make(ctx, src, pdfDir, out, page, tocOpts...); err != nil {
		slog.ErrorContext(ctx, "ошибка генерации содержания", slog.String("err", err.Error()))
		os.Exit(1)
	}
}
