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
	var Version bool

	flag.StringVar(&tf, "tf", "", "файл шаблона содержания (*.docx)")
	flag.StringVar(&td, "td", "", "папка для собранного содержания")
	flag.StringVar(&pd, "pd", "", "папка с PDF файлами")
	flag.IntVar(&tn, "tn", 3, "номер страницы содержания в собранном файле")
	flag.StringVar(&Level, "l", "error", "debug, info, warn, error")
	flag.BoolVar(&Version, "v", false, "версия программы")

	flag.Parse()
	ctx := context.Background()

	slogutil.Setup(Level)

	if Version {
		fmt.Println(version)
		return
	}

	var src, pdf, out string
	page := tn

	if tf != "" {
		// старый стиль: именованные флаги
		src = tf
		out = td
		pdf = pd
	} else {
		// новый стиль: позиционные аргументы
		args := flag.Args()
		if len(args) < 3 {
			slog.ErrorContext(ctx, "Обязательные аргументы: source-file output-folder pdf-folder [page]")
			os.Exit(1)
		}
		src = args[0]
		out = args[1]
		pdf = args[2]
		if len(args) > 3 {
			var err error
			page, err = strconv.Atoi(args[3])
			if err != nil {
				slog.ErrorContext(ctx, "page должен быть числом")
				os.Exit(1)
			}
		}
	}

	if src == "" || out == "" || pdf == "" {
		slog.ErrorContext(ctx, "Обязательные аргументы: source, output, pdf")
		os.Exit(1)
	}

	toc.Make(ctx, src, pdf, out, page)
}
