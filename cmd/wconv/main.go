package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	//"time"

	"golang.org/x/exp/slog"

	cache "github.com/Nemo08/ppdftb/pkg/cache"
	conv "github.com/Nemo08/ppdftb/pkg/convert"

	"github.com/alecthomas/kong"
)

var logLevels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
	"none":  -8,
}

var CLI struct {
	Src     []string `name:"source" short:"s" help:"файл или папка для конвертации" type:"*os.File"`
	Out     string   `name:"output" short:"o" help:"папка для сконвертированных *.pdf файлов" type:"existingdir" required:""`
	Outd    string   `name:"outputd" short:"d" help:"папка для собранных *.docx файлов" type:"existingdir" optional:""`
	Level   string   `name:"log" short:"l" help:"уровни логгирования: debug,info,warn,error" enum:"debug,info,warn,error" default:"error"`
	Version bool     `name:"version" short:"v" help:"версия программы"`

	Force bool `name:"force" short:"f" help:"конвертировать принудительно, игнорируя кэш"`

	Dx []string `name:"xml" short:"i" help:"данные для шаблона" type:"*os.File" optional:""`

	DxF string `name:"xmlf" short:"x" help:"корневая папка с файлами *.xml данных для шаблона" type:"existingdir" optional:""`
	DxL int    `name:"up" short:"u" help:"на сколько папок выше смотреть" optional:""`

	PicsDir string `name:"pics" short:"p" help:"папка с картинками для подстановки в шаблон" type:"existingdir" optional:""`
}

var (
	version string = "0.3j 2603"
)

func main() {
	_ = kong.Parse(&CLI)

	// Windows: путь вида "F:\path\" — финальный слэш экранирует закрывающую кавычку в CMD,
	// что приводит к некорректному парсингу аргументов. Убираем trailing слэш.
	CLI.Out = strings.TrimRight(CLI.Out, `/\`)
	if CLI.Outd != "" {
		CLI.Outd = strings.TrimRight(CLI.Outd, `/\`)
	}

	if CLI.Version {
		fmt.Println("version:", version)
	}

	if CLI.Level != "error" {
		//Установка логгера
		opts := &slog.HandlerOptions{
			Level:     logLevels[CLI.Level],
			AddSource: true,
		}

		logger := slog.New(slog.NewTextHandler(os.Stdout, opts))
		slog.SetDefault(logger)
	}

	var tempDir string = ""
	var err error

	if CLI.Outd == "" {
		tempDir, err = os.MkdirTemp(os.TempDir(), "wconv")
		//defer os.RemoveAll(tempDir)
		if err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
	} else {
		tempDir = CLI.Outd
	}
	ctx := context.Background()

	var data [][]byte
	_ = data
	var xmlPaths []string

	slog.Debug("xml пути", xmlPaths)

	// Сначала собираем файлы через -x (от верхних папок к нижним)
	if CLI.DxF != "" {
		xData, xPaths, err := conv.FindXMLFiles(CLI.DxF, CLI.DxL)
		if err != nil {
			slog.ErrorCtx(ctx, "Ошибка получения данных шаблона/шаблонов", err)
			os.Exit(1)
		}
		data = append(data, xData...)
		xmlPaths = append(xmlPaths, xPaths...)
	}

	// Затем добавляем явно указанные файлы через -i (они перекрывают -x)
	if len(CLI.Dx) != 0 {
		iData, err := conv.GetDataContent(ctx, CLI.Dx)
		if err != nil {
			slog.ErrorCtx(ctx, "Ошибка получения данных шаблона/шаблонов", err)
			os.Exit(1)
		}
		data = append(data, iData...)
		xmlPaths = append(xmlPaths, CLI.Dx...)
	}
	var mergedData []byte

	if len(data) > 0 {
		mergedData, err = conv.DataMerge(data)
		if err != nil {
			slog.ErrorCtx(ctx, "Ошибка данных", err)
			os.Exit(1)
		}
	}

	var toConvertList []string
	if CLI.Force {
		// Принудительная конвертация — собираем все файлы из источников минуя кэш.
		slog.Debug("принудительная конвертация, кэш игнорируется")
		toConvertList, err = conv.CollectWordFiles(CLI.Src)
		if err != nil {
			slog.Error("собрать файлы", slog.String("err", err.Error()))
			os.Exit(1)
		}
	} else {
		toConvertList, err = cache.FilesToConvert(CLI.Src, xmlPaths, CLI.Out, false)
		if err != nil {
			slog.Error("определить список файлов", slog.String("err", err.Error()))
			os.Exit(1)
		}
	}

	slog.Debug("изменившиеся файлы", toConvertList)

	if len(toConvertList) == 0 {
		fmt.Println("По моему мнению в папке", CLI.Src, "ничего не изменилось")
		fmt.Println("Файлы данных также не изменились: ", xmlPaths)
		fmt.Println("Ничего конвертировать не стану.")
		os.Exit(0)
	}

	err = conv.TplToDocxJJack3(ctx, toConvertList, tempDir, mergedData, CLI.PicsDir)
	if err != nil {
		slog.ErrorCtx(ctx, "Ошибка шаблонов", err)
		os.Exit(1)
	}

	err = conv.FilesToPdf(ctx, []string{tempDir}, CLI.Out)
	if err != nil {
		slog.ErrorCtx(ctx, "Ошибка конвертации", err)
		os.Exit(1)
	}

	if _, err = cache.CommitCache(append(CLI.Src, xmlPaths...), nil, false); err != nil {
		slog.Error("сохранить кэш", slog.String("err", err.Error()))
		os.Exit(1)
	}

}
