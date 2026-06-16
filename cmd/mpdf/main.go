package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"log/slog"

	"github.com/Nemo08/ppdftb/pkg/pdf"
	"github.com/Nemo08/ppdftb/pkg/slogutil"
)

var version string

func main() {
	flag.Parse()
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("mpdf", flag.ContinueOnError)
	var Dir, Out, Level string
	var Version, Appendix bool

	fs.StringVar(&Dir, "d", "", "папка с PDF файлами для объединения")
	fs.StringVar(&Out, "o", "out.pdf", "выходной PDF файл")
	fs.StringVar(&Level, "l", "error", "debug, info, warn, error")
	fs.BoolVar(&Version, "v", false, "версия программы")
	fs.BoolVar(&Appendix, "appendix", false,
		"распознавать файлы-разделители и нумеровать приложения буквами по ГОСТ Р 2.105-2019")

	if err := fs.Parse(args); err != nil {
		return 1
	}
	ctx := context.Background()
	slogutil.Setup(Level)

	if Version {
		fmt.Println(version)
		return 0
	}
	if Dir == "" {
		slog.ErrorContext(ctx, "Должна быть указана папка с PDF (-d)")
		return 1
	}

	var opts []pdf.MergeOption
	if Appendix {
		opts = append(opts, pdf.WithAppendix())
	}

	if err := pdf.Merge(ctx, Dir, Out, opts...); err != nil {
		slog.ErrorContext(ctx, err.Error())
		return 1
	}
	return 0
}
