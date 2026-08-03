//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"log/slog"

	"github.com/Nemo08/ppdftb/pkg/jobutil"
	"github.com/Nemo08/ppdftb/pkg/slogutil"
)

// hungProcessNames — исполняемые файлы COM-серверов, которые может оставить
// после себя аварийно завершённый engine2 (Word/AutoCAD не были закрыты
// через pool.Close(), Job Object не сработал).
var hungProcessNames = []string{"WINWORD.EXE", "ACAD.EXE"}

var version string

func main() {
	var (
		cfg       Config
		logLevel  string
		showVer   bool
	)

	flag.StringVar(&cfg.TplDir, "t", "Шаблон тома", "папка с DOCX-шаблонами")
	flag.StringVar(&cfg.TocTemplate, "tf", "", "файл шаблона содержания (*.docx)")
	flag.StringVar(&cfg.RootDir, "x", ".", "корневая папка с XML-данными")
	flag.StringVar(&cfg.OutFile, "o", "", "выходной PDF (пусто = авто из XML)")
	flag.StringVar(&cfg.PDFDir, "p", "", "временная папка PDF (по умолч. {root}/PDF)")
	flag.StringVar(&cfg.DocsDir, "d", "", "временная папка DOCX (по умолч. {root}/Документы тома)")
	flag.StringVar(&cfg.PicsDir, "pp", "", "папка с картинками для подстановки в шаблон (по умолч. {root}/pics)")
	flag.IntVar(&cfg.WordPool, "w", 4, "размер пула Word")
	flag.IntVar(&cfg.TocPageFrom, "tn", 4, "номер страницы оглавления в итоговом PDF")
	flag.IntVar(&cfg.PageFrom, "pf", 3, "номер первой страницы для нумерации")
	flag.IntVar(&cfg.NumberFrom, "nf", 3, "начальный номер для нумерации")
	flag.IntVar(&cfg.XMLDepth, "u", 0, "на сколько папок выше смотреть")
	flag.StringVar(&logLevel, "l", "error", "debug, info, warn, error")
	flag.BoolVar(&showVer, "v", false, "версия программы")

	flag.Parse()

	if showVer {
		fmt.Println("ppdftb engine2")
		if version != "" {
			fmt.Println(version)
		}
		return
	}

	// shutdown — в engine2 нет постоянного сервера (пул Word живёт только
	// на время одного запуска Run и закрывается через defer pool.Close()),
	// но если предыдущий запуск завершился аварийно (panic, kill, зависание),
	// COM-процессы Word/AutoCAD могут остаться висеть в системе.
	// shutdown находит и убивает такие зависшие процессы.
	if flag.NArg() > 0 && flag.Arg(0) == "shutdown" {
		slogutil.Setup(logLevel)
		pids := jobutil.FindProcessesByName(hungProcessNames...)
		if len(pids) == 0 {
			fmt.Println("Зависших процессов не найдено")
			return
		}
		jobutil.KillProcesses(pids)
		fmt.Printf("Остановлено зависших процессов: %d\n", len(pids))
		return
	}

	slogutil.Setup(logLevel)

	if err := Run(context.Background(), &cfg); err != nil {
		slog.Error("pipeline failed", slog.String("err", err.Error()))
		os.Exit(1)
	}
}
