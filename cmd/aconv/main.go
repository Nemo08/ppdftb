package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"log/slog"

	acadpool "github.com/Nemo08/ppdftb/pkg/acadpool"
	conv "github.com/Nemo08/ppdftb/pkg/convert"
	"github.com/Nemo08/ppdftb/pkg/fileutil"
	pdf "github.com/Nemo08/ppdftb/pkg/pdf"
	"github.com/Nemo08/ppdftb/pkg/slogutil"
)

// compile-time проверки.
var _ conv.CadConverter = (*acadpool.AcadPool)(nil)

type pdfMergerAdapter struct{}

func (pdfMergerAdapter) Merge(ctx context.Context, srcDir, dstFile string) error {
	return pdf.Merge(ctx, srcDir, dstFile)
}

var version string

func main() {
	var InputFile, InputDir, OutputDir, Level string
	var Version bool

	flag.StringVar(&InputFile, "if", "", "файл DWG/DXF для конвертации")
	flag.StringVar(&InputDir, "id", "", "папка с DWG/DXF файлами")
	flag.StringVar(&OutputDir, "od", "", "папка для сконвертированных PDF файлов")
	flag.StringVar(&Level, "log", "error", "debug, info, warn, error")
	flag.BoolVar(&Version, "v", false, "версия программы")

	flag.Parse()
	ctx := context.Background()

	slogutil.Setup(Level)

	if Version {
		fmt.Println(version)
		return
	}
	if OutputDir == "" {
		slog.ErrorContext(ctx, "Должна быть указана папка для PDF (-od)")
		os.Exit(1)
	}
	if InputFile == "" && InputDir == "" {
		slog.ErrorContext(ctx, "Должен быть указан входной файл (-if) или папка (-id)")
		os.Exit(1)
	}

	inputCadFiles, err := fileutil.CollectCadFiles(InputFile, InputDir)
	if err != nil {
		slog.ErrorContext(ctx, "Ошибка сбора DWG/DXF файлов", slog.String("err", err.Error()))
		os.Exit(1)
	}
	if len(inputCadFiles) == 0 {
		slog.DebugContext(ctx, "Нет DWG/DXF файлов для конвертации")
		return
	}

	pool := acadpool.NewAcadPool(1)
	defer pool.Close()

	if err := runA2pdf(ctx, pool, inputCadFiles, OutputDir); err != nil {
		slog.ErrorContext(ctx, "Ошибка конвертации", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

// runA2pdf выполняет конвертацию через пул и возвращает ошибку вместо os.Exit,
// чтобы defer pool.Close() в main успевал закрыть пул.
func runA2pdf(ctx context.Context, pool conv.CadConverter, files []string, outputDir string) error {
	return conv.A2pdfWithPool(ctx, pool, pdfMergerAdapter{}, files, outputDir)
}
