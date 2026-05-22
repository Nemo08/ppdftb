// wconv — Word converter. Конвертирует .doc/.docx/.rtf в PDF через Microsoft Word
// (COM-автоматизация). Поддерживает шаблоны JJack, подстановку данных из XML/JSON
// и встраивание изображений. Работает только на Windows.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"log/slog"

	cache "github.com/Nemo08/ppdftb/pkg/cache"
	conv "github.com/Nemo08/ppdftb/pkg/convert"
	"github.com/Nemo08/ppdftb/pkg/slogutil"
)

var CLI struct {
	Src     []string `name:"source" short:"s" help:"файл или папка для конвертации" type:"*os.File"`
	Out     string   `name:"output" short:"o" help:"папка для сконвертированных *.pdf файлов" type:"existingdir" optional:""`
	Outd    string   `name:"outputd" short:"d" help:"папка для собранных *.docx файлов" type:"existingdir" optional:""`
	Level   string   `name:"log" short:"l" help:"уровни логгирования: debug,info,warn,error" enum:"debug,info,warn,error" default:"error"`
	Version bool     `name:"version" short:"v" help:"версия программы"`

	UseCache bool `name:"cache" short:"c" help:"использовать кэш для пропуска неизменившихся файлов"`

	Dx []string `name:"xml" short:"i" help:"данные для шаблона" type:"*os.File" optional:""`

	DxF string `name:"xmlf" short:"x" help:"корневая папка с файлами *.xml данных для шаблона" type:"existingdir" optional:""`
	DxL int    `name:"up" short:"u" help:"на сколько папок выше смотреть" optional:""`

	PicsDir string `name:"pics" short:"p" help:"папка с картинками для подстановки в шаблон" type:"existingdir" optional:""`
}

var version string

func main() {
	_ = kong.Parse(&CLI)

	// Windows: путь вида "F:\path\" — финальный слэш экранирует закрывающую кавычку в CMD,
	// что приводит к некорректному парсингу аргументов. Убираем trailing слэш.
	CLI.Out = strings.TrimRight(CLI.Out, `/\`)
	if CLI.Outd != "" {
		CLI.Outd = strings.TrimRight(CLI.Outd, `/\`)
	}

	if CLI.Version {
		fmt.Println(version)
		return
	}
	if CLI.Out == "" {
		slog.Error("Должна быть указана папка для PDF (--output)")
		os.Exit(1)
	}

	slogutil.Setup(CLI.Level)

	var tempDir string = ""
	var err error

	if CLI.Outd == "" {
		tempDir, err = os.MkdirTemp(os.TempDir(), "wconv")
		defer os.RemoveAll(tempDir)
		if err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
	} else {
		tempDir = CLI.Outd
	}
	ctx := context.Background()

	var data [][]byte
	var xmlPaths []string

	slog.Debug("xml пути", slog.Any("xmlPaths", xmlPaths))

	// Сначала собираем файлы через -x (от верхних папок к нижним)
	if CLI.DxF != "" {
		xData, xPaths, err := conv.FindXMLFiles(CLI.DxF, CLI.DxL)
		if err != nil {
			slog.ErrorContext(ctx, "Ошибка получения данных шаблона/шаблонов", slog.Any("err", err))
			os.Exit(1)
		}
		data = append(data, xData...)
		xmlPaths = append(xmlPaths, xPaths...)
	}

	// Затем добавляем явно указанные файлы через -i (они перекрывают -x)
	if len(CLI.Dx) != 0 {
		iData, err := conv.GetDataContent(ctx, CLI.Dx)
		if err != nil {
			slog.ErrorContext(ctx, "Ошибка получения данных шаблона/шаблонов", slog.Any("err", err))
			os.Exit(1)
		}
		data = append(data, iData...)
		xmlPaths = append(xmlPaths, CLI.Dx...)
	}
	var mergedData []byte

	if len(data) > 0 {
		mergedData, err = conv.DataMerge(data)
		if err != nil {
			slog.ErrorContext(ctx, "Ошибка данных", slog.Any("err", err))
			os.Exit(1)
		}
	}

	slog.Debug("собранные данные", slog.String("data", string(mergedData)))

	var toConvertList []string
	if CLI.UseCache {
		// Кэш включён — конвертируем только изменившиеся файлы.
		slog.Debug("кэш включён, проверяем изменения")
		toConvertList, err = cache.FilesToConvert(CLI.Src, xmlPaths, CLI.Out, false)
		if err != nil {
			slog.Error("определить список файлов", slog.String("err", err.Error()))
			os.Exit(1)
		}
	} else {
		// Кэш выключен (по умолчанию) — собираем все файлы.
		slog.Debug("кэш выключен, конвертируем всё")
		toConvertList, err = conv.CollectWordFiles(CLI.Src)
		if err != nil {
			slog.Error("собрать файлы", slog.String("err", err.Error()))
			os.Exit(1)
		}
	}

	slog.Debug("изменившиеся файлы", slog.Any("toConvertList", toConvertList))

	if len(toConvertList) == 0 {
		if CLI.UseCache {
			fmt.Println("Файлы не изменились, конвертировать нечего.")
		} else {
			fmt.Println("Нет файлов для конвертации в", CLI.Src)
		}
		os.Exit(0)
	}

	err = conv.TplToDocxJJack3(ctx, toConvertList, tempDir, mergedData, CLI.PicsDir)
	if err != nil {
		slog.ErrorContext(ctx, "Ошибка шаблонов", slog.Any("err", err))
		os.Exit(1)
	}

	err = conv.FilesToPdf(ctx, []string{tempDir}, CLI.Out)
	if err != nil {
		slog.ErrorContext(ctx, "Ошибка конвертации", slog.Any("err", err))
		os.Exit(1)
	}

	if CLI.UseCache {
		if _, err = cache.CommitCache(append(CLI.Src, xmlPaths...), nil, false); err != nil {
			slog.Error("сохранить кэш", slog.String("err", err.Error()))
			os.Exit(1)
		}
	}

}
