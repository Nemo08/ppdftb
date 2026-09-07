//go:build windows

// Package wordpool предоставляет пул экземпляров Microsoft Word для
// параллельной конвертации DOCX в PDF через COM-интерфейс.
package wordpool

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"log/slog"

	"github.com/Nemo08/ppdftb/pkg/olepool"
	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

var wordCfg = olepool.Config{
	AppName: "Word.Application",
	Setup: func(app *ole.IDispatch) {
		applyWordUISettings(app)
	},
}

// Прямые свойства Application. Ошибка применения не критична — конвертация
// всё равно пройдёт, но пользователь может увидеть диалоги/мерцание окна.
var wordAppSettings = []struct {
	name  string
	value any
}{
	{"Visible", false},
	{"DisplayAlerts", 0},
	{"ScreenUpdating", false},
	{"AutomationSecurity", 3},
}

// Свойства Application.Options. Ставятся через объект Options, а не через
// dotted-имя на Application (PutProperty(app, "Options.X") даёт «Неизвестное имя.»).
var wordOptionsSettings = []struct {
	name  string
	value any
}{
	{"CheckSpellingAsYouType", false},
	{"CheckGrammarAsYouType", false},
	{"SavePropertiesPrompt", false},
	{"BackgroundSave", false},
	// Не ждать фоновой печати / недоступных принтеров при открытии.
	{"PrintBackground", false},
	{"BackgroundPrint", false},
}

// virtualPrinters — локальные виртуальные принтеры, которые не требуют
// «Дождитесь подключения принтера». Пробуем по очереди.
var virtualPrinters = []string{
	"Microsoft Print to PDF",
	"Microsoft XPS Document Writer",
}

func applyWordUISettings(app *ole.IDispatch) {
	for _, s := range wordAppSettings {
		if _, err := oleutil.PutProperty(app, s.name, s.value); err != nil {
			slog.Warn("не удалось применить настройку Word",
				slog.String("prop", s.name), slog.String("err", err.Error()))
		}
	}

	// Обязательно ДО любой работы с документами: иначе Word на Win11
	// может минутами висеть на диалоге «Дождитесь подключения принтера»
	// (дефолтный/сетевой принтер недоступен).
	setVirtualPrinter(app)

	optDisp, err := oleutil.GetProperty(app, "Options")
	if err != nil {
		slog.Warn("не удалось получить Word.Options", slog.String("err", err.Error()))
		return
	}
	opts := optDisp.ToIDispatch()
	defer opts.Release()

	for _, s := range wordOptionsSettings {
		if _, err := oleutil.PutProperty(opts, s.name, s.value); err != nil {
			slog.Warn("не удалось применить настройку Word",
				slog.String("prop", "Options."+s.name), slog.String("err", err.Error()))
		}
	}
}

func setVirtualPrinter(app *ole.IDispatch) {
	for _, name := range virtualPrinters {
		if _, err := oleutil.PutProperty(app, "ActivePrinter", name); err == nil {
			slog.Debug("ActivePrinter установлен", slog.String("printer", name))
			return
		} else {
			slog.Debug("ActivePrinter не подошёл",
				slog.String("printer", name), slog.String("err", err.Error()))
		}
	}
	slog.Warn("не удалось выставить виртуальный принтер — возможны зависания на «Дождитесь подключения принтера»")
}

type wordJob struct {
	fromFile string
	toFile   string
}

func (j *wordJob) Process(app *ole.IDispatch) error {
	docsDisp, err := oleutil.GetProperty(app, "Documents")
	if err != nil {
		return fmt.Errorf("documents: %w", err)
	}
	docs := docsDisp.ToIDispatch()
	defer docs.Release()

	wordFilev, err := oleutil.CallMethod(docs, "Open",
		j.fromFile, false, true, false,
	)
	if err != nil {
		return fmt.Errorf("открыть %q: %w", filepath.Base(j.fromFile), err)
	}
	wordFile := wordFilev.ToIDispatch()
	defer func() {
		if _, err := oleutil.CallMethod(wordFile, "Close", false); err != nil {
			slog.Error("Close document", slog.String("err", err.Error()))
		}
		wordFile.Release()
	}()

	_, err = oleutil.CallMethod(wordFile, "ExportAsFixedFormat",
		j.toFile, 17, false, 0, 0, 0, 0, 0,
		true, true, 1, true, true, false,
	)
	if err != nil {
		return fmt.Errorf("ExportAsFixedFormat %q: %w", filepath.Base(j.fromFile), err)
	}

	slog.Debug("сконвертирован", slog.String("file", filepath.Base(j.toFile)))
	return nil
}

// WordPool — пул экземпляров Word для параллельной конвертации.
type WordPool struct {
	pool *olepool.Pool
}

// NewWordPool создаёт пул из size экземпляров Word.
func NewWordPool(size int) *WordPool {
	return &WordPool{pool: olepool.NewPool(size, wordCfg)}
}

// WaitReady блокируется пока хотя бы один воркер Word не будет готов.
func (p *WordPool) WaitReady(ctx context.Context) error {
	return p.pool.WaitReady(ctx)
}

// WordToPdf конвертирует один файл через пул.
func (p *WordPool) WordToPdf(ctx context.Context, fromWordFile, toPdfFile string) error {
	fromFile, err := filepath.Abs(strings.TrimSpace(fromWordFile))
	if err != nil {
		return fmt.Errorf("абсолютный путь источника: %w", err)
	}
	toFile, err := filepath.Abs(strings.TrimSpace(toPdfFile))
	if err != nil {
		return fmt.Errorf("абсолютный путь назначения: %w", err)
	}
	return p.pool.Submit(ctx, &wordJob{fromFile: fromFile, toFile: toFile})
}

// Close завершает все экземпляры Word и освобождает ресурсы.
func (p *WordPool) Close() {
	p.pool.Close()
}
