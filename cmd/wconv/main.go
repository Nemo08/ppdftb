package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"log/slog"

	cache "github.com/Nemo08/ppdftb/pkg/cache"
	conv "github.com/Nemo08/ppdftb/pkg/convert"
	"github.com/Nemo08/ppdftb/pkg/slogutil"
)

type stringSlice []string

func (s *stringSlice) String() string { return "" }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

var version string

func main() {
	var Src, Out, Outd, Level string
	var Version, UseCache bool
	var DxF, PicsDir string
	var DxL int
	var Dx stringSlice

	flag.StringVar(&Src, "s", "", "файл или папка для конвертации")
	flag.StringVar(&Out, "o", "", "папка для сконвертированных *.pdf файлов")
	flag.StringVar(&Outd, "d", "", "папка для собранных *.docx файлов")
	flag.StringVar(&Level, "l", "error", "debug, info, warn, error")
	flag.BoolVar(&Version, "v", false, "версия программы")
	flag.BoolVar(&UseCache, "c", false, "использовать кэш (-c)")
	flag.Var(&Dx, "i", "данные для шаблона (файл .xml/.json)")
	flag.StringVar(&DxF, "x", "", "корневая папка с файлами *.xml данных для шаблона")
	flag.IntVar(&DxL, "u", 0, "на сколько папок выше смотреть")
	flag.StringVar(&PicsDir, "p", "", "папка с картинками для подстановки в шаблон")

	flag.Parse()

	// Windows: путь вида "F:\path\" — финальный слэш экранирует закрывающую кавычку в CMD,
	// что приводит к некорректному парсингу аргументов. Убираем trailing слэш.
	Out = strings.TrimRight(Out, `/\`)
	if Outd != "" {
		Outd = strings.TrimRight(Outd, `/\`)
	}

	if Version {
		fmt.Println(version)
		return
	}
	if Out == "" {
		slog.Error("Должна быть указана папка для PDF (-o)")
		os.Exit(1)
	}

	slogutil.Setup(Level)

	var tempDir string = ""
	var err error

	if Outd == "" {
		tempDir, err = os.MkdirTemp(os.TempDir(), "wconv")
		defer os.RemoveAll(tempDir)
		if err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
	} else {
		tempDir = Outd
	}
	ctx := context.Background()

	var data [][]byte
	var xmlPaths []string

	slog.Debug("xml пути", slog.Any("xmlPaths", xmlPaths))

	// Сначала собираем файлы через -x (от верхних папок к нижним)
	if DxF != "" {
		xData, xPaths, err := conv.FindXMLFiles(DxF, DxL)
		if err != nil {
			slog.ErrorContext(ctx, "Ошибка получения данных шаблона/шаблонов", slog.Any("err", err))
			os.Exit(1)
		}
		data = append(data, xData...)
		xmlPaths = append(xmlPaths, xPaths...)
	}

	// Затем добавляем явно указанные файлы через -i (они перекрывают -x)
	if len(Dx) != 0 {
		iData, err := conv.GetDataContent(ctx, Dx)
		if err != nil {
			slog.ErrorContext(ctx, "Ошибка получения данных шаблона/шаблонов", slog.Any("err", err))
			os.Exit(1)
		}
		data = append(data, iData...)
		xmlPaths = append(xmlPaths, Dx...)
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
	sources := []string{Src}
	if UseCache {
		// Кэш включён — конвертируем только изменившиеся файлы.
		slog.Debug("кэш включён, проверяем изменения")
		toConvertList, err = cache.FilesToConvert(sources, xmlPaths, Out, false)
		if err != nil {
			slog.Error("определить список файлов", slog.String("err", err.Error()))
			os.Exit(1)
		}
	} else {
		// Кэш выключен (по умолчанию) — собираем все файлы.
		slog.Debug("кэш выключен, конвертируем всё")
		toConvertList, err = conv.CollectWordFiles(sources)
		if err != nil {
			slog.Error("собрать файлы", slog.String("err", err.Error()))
			os.Exit(1)
		}
	}

	slog.Debug("изменившиеся файлы", slog.Any("toConvertList", toConvertList))

	if len(toConvertList) == 0 {
		if UseCache {
			fmt.Println("Файлы не изменились, конвертировать нечего.")
		} else {
			fmt.Println("Нет файлов для конвертации в", Src)
		}
		os.Exit(0)
	}

	err = conv.TplToDocxJJack3(ctx, toConvertList, tempDir, mergedData, PicsDir)
	if err != nil {
		slog.ErrorContext(ctx, "Ошибка шаблонов", slog.Any("err", err))
		os.Exit(1)
	}

	err = conv.FilesToPdf(ctx, []string{tempDir}, Out)
	if err != nil {
		slog.ErrorContext(ctx, "Ошибка конвертации", slog.Any("err", err))
		os.Exit(1)
	}

	if UseCache {
		if _, err = cache.CommitCache(sources, nil, false); err != nil {
			slog.Error("сохранить кэш", slog.String("err", err.Error()))
			os.Exit(1)
		}
	}
}
