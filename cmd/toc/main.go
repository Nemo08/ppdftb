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

// extractTrailingFlags извлекает из args флаги, которые могут стоять ПОСЛЕ
// позиционных аргументов. Go flag.Parse() останавливается на первом
// позиционном аргументе, поэтому такие флаги нужно вынимать вручную.
// Возвращает очищенный список args, признак -appendix и значение -l.
func extractTrailingFlags(args []string) (clean []string, appendix bool, logLevel string) {
	clean = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-appendix", "--appendix":
			appendix = true
		case "-l", "--l":
			if i+1 < len(args) {
				logLevel = args[i+1]
				i++
			} else {
				clean = append(clean, args[i])
			}
		default:
			clean = append(clean, args[i])
		}
	}
	return clean, appendix, logLevel
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

	// Сканируем -appendix и -l вручную, потому что Go flag.Parse()
	// останавливается на первом позиционном аргументе и хвостовые флаги
	// попали бы в fs.Args() как "фантомные" позиционные.
	args, appendix, logLevel := extractTrailingFlags(args)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	ctx := context.Background()

	if logLevel != "" {
		Level = logLevel
	}
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
		var code int
		src, out, pdfDir, page, code = parsePositionalArgs(ctx, fs.Args(), page)
		if code != 0 {
			return code
		}
	}

	if src == "" || out == "" || pdfDir == "" {
		slog.ErrorContext(ctx, "Обязательные аргументы: source, output, pdf")
		return 1
	}

	return buildToc(ctx, src, pdfDir, out, page, appendix)
}

// buildToc генерирует содержание с опциями и возвращает код выхода.
func buildToc(ctx context.Context, src, pdfDir, out string, page int, appendix bool) int {
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

// parsePositionalArgs разбирает позиционные аргументы
// source-file output-folder pdf-folder [page]. Возвращает код ошибки
// (0 — успех, 1 — ошибка аргументов).
func parsePositionalArgs(ctx context.Context, posArgs []string, page int) (src, out, pdfDir string, pageOut int, code int) {
	if len(posArgs) < 3 {
		slog.ErrorContext(ctx, "Обязательные аргументы: source-file output-folder pdf-folder [page]")
		return "", "", "", page, 1
	}
	pageOut = page
	if len(posArgs) > 3 {
		var err error
		pageOut, err = strconv.Atoi(posArgs[3])
		if err != nil {
			slog.ErrorContext(ctx, "page должен быть числом")
			return "", "", "", page, 1
		}
	}
	return posArgs[0], posArgs[1], posArgs[2], pageOut, 0
}
