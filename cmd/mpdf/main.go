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
	var Dir, Out, Level string
	var Version, Appendix bool

	flag.StringVar(&Dir, "d", "", "папка с PDF файлами для объединения")
	flag.StringVar(&Out, "o", "out.pdf", "выходной PDF файл")
	flag.StringVar(&Level, "l", "error", "debug, info, warn, error")
	flag.BoolVar(&Version, "v", false, "версия программы")
	flag.BoolVar(&Appendix, "appendix", false,
		"распознавать файлы-разделители и нумеровать приложения буквами по ГОСТ Р 2.105-2019")

	flag.Parse()
	ctx := context.Background()
	slogutil.Setup(Level)

	if Version {
		fmt.Println(version)
		return
	}
	if Dir == "" {
		slog.ErrorContext(ctx, "Должна быть указана папка с PDF (-d)")
		os.Exit(1)
	}

	var opts []pdf.MergeOption
	if Appendix {
		opts = append(opts, pdf.WithAppendix())
	}

	if err := pdf.Merge(ctx, Dir, Out, opts...); err != nil {
		slog.ErrorContext(ctx, err.Error())
		os.Exit(1)
	}
}
