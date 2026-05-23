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
	"github.com/Nemo08/ppdftb/pkg/wordpool"
)

// compile-time проверки.
var _ conv.WordConverter = (*wordpool.WordPool)(nil)
var _ conv.ConvCache = cache.ConvCache{}

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

	if Version {
		fmt.Println(version)
		return
	}
	if Out == "" {
		slog.Error("Должна быть указана папка для PDF (-o)")
		os.Exit(1)
	}

	slogutil.Setup(Level)

	p := &conv.WconvPipeline{
		Src:      Src,
		Out:      strings.TrimRight(Out, `/\`),
		Outd:     strings.TrimRight(Outd, `/\`),
		DxFlags:  Dx,
		DxF:      DxF,
		DxL:      DxL,
		PicsDir:  PicsDir,
		UseCache: UseCache,
		Cache:    nil,
	}
	if UseCache {
		p.Cache = cache.ConvCache{}
	}

	pool := wordpool.NewWordPool(4)
	defer pool.Close()

	if err := conv.RunWconvWithPool(context.Background(), pool, p); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
