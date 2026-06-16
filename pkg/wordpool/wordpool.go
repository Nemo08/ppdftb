//go:build windows

package wordpool

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Nemo08/ppdftb/pkg/olepool"
	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"log/slog"
)

var wordCfg = olepool.Config{
	AppName: "Word.Application",
	Setup: func(app *ole.IDispatch) {
		_, _ = oleutil.PutProperty(app, "Visible", false)
		_, _ = oleutil.PutProperty(app, "DisplayAlerts", 0)
		_, _ = oleutil.PutProperty(app, "ScreenUpdating", false)
		_, _ = oleutil.PutProperty(app, "AutomationSecurity", 3)
		_, _ = oleutil.PutProperty(app, "Options.CheckSpellingAsYouType", false)
		_, _ = oleutil.PutProperty(app, "Options.CheckGrammarAsYouType", false)
		_, _ = oleutil.PutProperty(app, "Options.SavePropertiesPrompt", false)
		_, _ = oleutil.PutProperty(app, "Options.BackgroundSave", false)
	},
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
