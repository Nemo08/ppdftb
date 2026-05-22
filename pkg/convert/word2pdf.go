//go:build windows

package convert

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
	"log/slog"
)

// wordWorker — один экземпляр Word, работающий в выделенном OS-потоке.
type wordWorker struct {
	jobs     chan wordJob
	done     chan struct{}
	pool     *WordPool
	wordPIDs []uint32
}

type wordJob struct {
	fromFile string
	toFile   string
	result   chan error
}

// WordPool — пул экземпляров Word для параллельной конвертации.
type WordPool struct {
	workers   []*wordWorker
	jobs      chan wordJob
	once      sync.Once
	jobHandle windows.Handle
}

// NewWordPool создаёт пул из size экземпляров Word и запускает их.
// Каждый экземпляр работает в своём OS-потоке (требование COM).
func NewWordPool(size int) *WordPool {
	if size <= 0 {
		size = 1
	}
	p := &WordPool{
		jobs:      make(chan wordJob, size*4),
		workers:   make([]*wordWorker, size),
		jobHandle: createJobObject(),
	}
	for i := range p.workers {
		w := &wordWorker{
			jobs: p.jobs,
			done: make(chan struct{}),
			pool: p,
		}
		p.workers[i] = w
		go w.run()
	}
	slog.Debug("WordPool запущен", slog.Int("workers", size))
	return p
}

// WordToPdf конвертирует один файл через пул.
func (p *WordPool) WordToPdf(ctx context.Context, fromWordFile, toPdfFile string) error {
	fromFile, err := filepath.Abs(fromWordFile)
	if err != nil {
		return fmt.Errorf("абсолютный путь источника: %w", err)
	}
	toFile, err := filepath.Abs(toPdfFile)
	if err != nil {
		return fmt.Errorf("абсолютный путь назначения: %w", err)
	}

	result := make(chan error, 1)
	job := wordJob{fromFile: fromFile, toFile: toFile, result: result}

	select {
	case p.jobs <- job:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close завершает все экземпляры Word и освобождает ресурсы.
func (p *WordPool) Close() {
	p.once.Do(func() {
		close(p.jobs)
		for _, w := range p.workers {
			<-w.done
		}
		// Принудительно убиваем оставшиеся процессы Word (если Quit() не сработал).
		for _, w := range p.workers {
			killProcesses(w.wordPIDs)
		}
		if p.jobHandle != 0 {
			windows.CloseHandle(p.jobHandle)
			p.jobHandle = 0
		}
		slog.Debug("WordPool остановлен")
	})
}

// run — главный цикл воркера. Весь COM живёт внутри этой горутины.
func (w *wordWorker) run() {
	defer close(w.done)

	// Привязываем горутину к OS-потоку — обязательно для COM.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Инициализируем COM. COINIT_APARTMENTTHREADED — единственно верный
	// режим для автоматизации Office (MULTITHREADED с Word не работает).
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		if oleErr, ok := err.(*ole.OleError); !ok || oleErr.Code() != 0x00000001 { // S_FALSE — уже инициализировано
			slog.Error("CoInitializeEx", slog.String("err", err.Error()))
			// Сигнализируем о сбое всем ожидающим задачам.
			for job := range w.jobs {
				job.result <- fmt.Errorf("CoInitializeEx: %w", err)
			}
			return
		}
	}
	defer ole.CoUninitialize()

	// Снимок PID до создания COM-объекта (для Job Object).
	var beforePIDs []uint32
	if w.pool != nil && w.pool.jobHandle != 0 {
		beforePIDs = getAllPids()
	}

	// Создаём Word.Application — один раз на весь жизненный цикл воркера.
	unknown, err := oleutil.CreateObject("Word.Application")
	if err != nil {
		slog.Error("создать Word.Application", slog.String("err", err.Error()))
		for job := range w.jobs {
			job.result <- fmt.Errorf("Word.Application: %w", err)
		}
		return
	}

	word, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		slog.Error("QueryInterface", slog.String("err", err.Error()))
		unknown.Release()
		for job := range w.jobs {
			job.result <- fmt.Errorf("QueryInterface: %w", err)
		}
		return
	}
	unknown.Release()
	defer word.Release()

	// Привязываем новый процесс Word к Job Object + сохраняем PID для принудительного убийства.
	if w.pool != nil {
		afterPIDs := getAllPids()
		assignPidsToJob(w.pool.jobHandle, beforePIDs, afterPIDs)
		w.wordPIDs = collectNewPids(beforePIDs, afterPIDs)
	}

	// Настраиваем Word.
	oleutil.PutProperty(word, "Visible", false)        // скрываем окно
	oleutil.PutProperty(word, "DisplayAlerts", 0)      // подавляем диалоги
	oleutil.PutProperty(word, "ScreenUpdating", false) // отключаем перерисовку
	oleutil.PutProperty(word, "AutomationSecurity", 3) // отключаем макросы
	oleutil.PutProperty(word, "Options.CheckSpellingAsYouType", false)
	oleutil.PutProperty(word, "Options.CheckGrammarAsYouType", false)
	oleutil.PutProperty(word, "Options.SavePropertiesPrompt", false)
	oleutil.PutProperty(word, "Options.BackgroundSave", false)

	docsDisp, err := oleutil.GetProperty(word, "Documents")
	if err != nil {
		slog.Error("получить Documents", slog.String("err", err.Error()))
		for job := range w.jobs {
			job.result <- fmt.Errorf("Documents: %w", err)
		}
		return
	}
	docs := docsDisp.ToIDispatch()
	defer docs.Release()

	slog.Debug("Word воркер готов")

	// Обрабатываем задания пока канал открыт.
	for job := range w.jobs {
		job.result <- convertFile(word, docs, job.fromFile, job.toFile)
	}

	// Завершаем Word.
	if _, err := oleutil.CallMethod(word, "Quit"); err != nil {
		slog.Error("Word Quit", slog.String("err", err.Error()))
	}
}

// convertFile выполняет конвертацию одного документа.
func convertFile(word, docs *ole.IDispatch, fromFile, toFile string) error {
	// Открываем документ.
	// Аргументы Open: FileName, ConfirmConversions, ReadOnly, AddToRecentFiles
	wordFilev, err := oleutil.CallMethod(docs, "Open",
		fromFile, // FileName
		false,    // ConfirmConversions
		true,     // ReadOnly — не блокируем файл
		false,    // AddToRecentFiles
	)
	if err != nil {
		return fmt.Errorf("открыть %q: %w", filepath.Base(fromFile), err)
	}
	wordFile := wordFilev.ToIDispatch()
	defer func() {
		// Закрываем без сохранения (wdDoNotSaveChanges).
		if _, err := oleutil.CallMethod(wordFile, "Close", false); err != nil {
			slog.Error("Close document", slog.String("err", err.Error()))
		}
		wordFile.Release()
	}()

	// Экспортируем в PDF.
	// Документация: https://learn.microsoft.com/ru-ru/dotnet/api/microsoft.office.interop.word._document.exportasfixedformat
	_, err = oleutil.CallMethod(wordFile, "ExportAsFixedFormat",
		toFile, // OutputFileName
		17,     // ExportFormat = wdExportFormatPDF
		false,  // OpenAfterExport
		0,      // OptimizeFor = wdExportOptimizeForPrint
		0,      // Range = wdExportAllDocument
		0,      // From
		0,      // To
		0,      // Item = wdExportDocumentContent
		true,   // IncludeDocProps
		true,   // KeepIRM
		1,      // CreateBookmarks = wdExportCreateWordBookmarks
		true,   // DocStructureTags
		true,   // BitmapMissingFonts
		false,  // UseISO19005_1 (PDF/A)
	)
	if err != nil {
		return fmt.Errorf("ExportAsFixedFormat %q: %w", filepath.Base(fromFile), err)
	}

	slog.Debug("сконвертирован", slog.String("file", filepath.Base(toFile)))
	return nil
}
