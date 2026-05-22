//go:build windows

package convert

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"log/slog"

	pdf "github.com/Nemo08/ppdftb/pkg/pdf"
	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
)

var (
	replacesMu sync.RWMutex
	replaces   = map[string]string{
		"Name":   "Имя",
		"Number": "66955",
	}
)

// StrReplace заменяет плейсхолдеры вида {{Name}}/{{Number}} в строке.
func StrReplace(in string) string {
	replacesMu.RLock()
	defer replacesMu.RUnlock()
	s := in
	for k, v := range replaces {
		if strings.Contains(s, "\\{\\{"+k+"\\}\\}") {
			s = strings.ReplaceAll(s, "\\{\\{"+k+"\\}\\}", v)
		}
	}
	return s
}

// --- AcadPool ---

type acadJob struct {
	fromFile string
	toDir    string
	result   chan error
}

type acadWorker struct {
	jobs      chan acadJob
	done      chan struct{}
	pool      *AcadPool
	acadPIDs []uint32
}

// AcadPool — пул экземпляров AutoCAD для параллельной конвертации DWG/DXF в PDF.
// Каждый экземпляр работает в своём OS-потоке (требование COM).
type AcadPool struct {
	workers   []*acadWorker
	jobs      chan acadJob
	once      sync.Once
	jobHandle windows.Handle
}

// NewAcadPool создаёт пул из size экземпляров AutoCAD.
func NewAcadPool(size int) *AcadPool {
	if size <= 0 {
		size = 1
	}
	p := &AcadPool{
		jobs:      make(chan acadJob, size*4),
		workers:   make([]*acadWorker, size),
		jobHandle: createJobObject(),
	}
	for i := range p.workers {
		w := &acadWorker{
			jobs: p.jobs,
			done: make(chan struct{}),
			pool: p,
		}
		p.workers[i] = w
		go w.run()
	}
	slog.Debug("AcadPool запущен", slog.Int("workers", size))
	return p
}

// AcadToPdf конвертирует один DWG/DXF через пул.
func (p *AcadPool) AcadToPdf(ctx context.Context, fromFile, toDir string) error {
	fromFile, err := filepath.Abs(fromFile)
	if err != nil {
		return err
	}
	toDir, err = filepath.Abs(toDir)
	if err != nil {
		return err
	}

	result := make(chan error, 1)
	job := acadJob{fromFile: fromFile, toDir: toDir, result: result}

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

// Close завершает все экземпляры AutoCAD и освобождает ресурсы.
func (p *AcadPool) Close() {
	p.once.Do(func() {
		close(p.jobs)
		for _, w := range p.workers {
			<-w.done
		}
		for _, w := range p.workers {
			killProcesses(w.acadPIDs)
		}
		if p.jobHandle != 0 {
			windows.CloseHandle(p.jobHandle)
			p.jobHandle = 0
		}
		slog.Debug("AcadPool остановлен")
	})
}

func (w *acadWorker) run() {
	defer close(w.done)

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		slog.Error("Acad CoInitializeEx", slog.String("err", err.Error()))
		for job := range w.jobs {
			job.result <- err
		}
		return
	}
	defer ole.CoUninitialize()

	// Снимок PID до создания COM-объекта (для Job Object).
	var beforePIDs []uint32
	if w.pool != nil && w.pool.jobHandle != 0 {
		beforePIDs = getAllPids()
	}

	unknown, err := oleutil.CreateObject("AutoCAD.Application")
	if err != nil {
		slog.Error("создать AutoCAD.Application", slog.String("err", err.Error()))
		for job := range w.jobs {
			job.result <- err
		}
		return
	}

	acad, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		slog.Error("Acad QueryInterface", slog.String("err", err.Error()))
		unknown.Release()
		for job := range w.jobs {
			job.result <- err
		}
		return
	}

	// Привязываем новый процесс AutoCAD к Job Object + сохраняем PID.
	if w.pool != nil {
		afterPIDs := getAllPids()
		assignPidsToJob(w.pool.jobHandle, beforePIDs, afterPIDs)
		w.acadPIDs = collectNewPids(beforePIDs, afterPIDs)
	}

	slog.Debug("Acad воркер готов")

	for job := range w.jobs {
		job.result <- processFile(acad, job.fromFile, job.toDir)
	}

	if _, err := oleutil.CallMethod(acad, "Quit"); err != nil {
		slog.Error("Acad Quit", slog.String("err", err.Error()))
	}
	acad.Release()
	unknown.Release()
}

// processFile выполняет конвертацию одного DWG/DXF в PDF через активный экземпляр AutoCAD.
func processFile(acad *ole.IDispatch, fromFile, toDir string) error {
	slog.Debug("acadToPdf " + fromFile + " " + toDir)

	docsv, err := acad.GetProperty("Documents")
	if err != nil {
		return err
	}
	docs := docsv.ToIDispatch()
	defer docs.Release()

	openArguments := []interface{}{fromFile, true}
	cadFilev, err := docs.CallMethod("Open", openArguments...)
	if err != nil {
		return err
	}
	cadFile := cadFilev.ToIDispatch()
	defer cadFile.Release()

	activeDocv, err := acad.GetProperty("ActiveDocument")
	if err != nil {
		return err
	}
	activeDoc := activeDocv.ToIDispatch()
	defer activeDoc.Release()

	//Modelspace replace
	spaces := []string{"ModelSpace", "PaperSpace"}
	for _, spaceName := range spaces {
		slog.Debug(spaceName + " replaces begin")
		msv, err := activeDoc.GetProperty(spaceName)
		if err != nil {
			return err
		}
		ms := msv.ToIDispatch()
		defer ms.Release()

		msCount, err := ms.GetProperty("Count")
		if err != nil {
			return err
		}
		for i := int32(0); i < msCount.Value().(int32); i++ {
			itemv, err := ms.CallMethod("Item", []interface{}{i}...)
			if err != nil {
				return err
			}
			item := itemv.ToIDispatch()

			ts, err := item.GetProperty("TextString")
			if err == nil {
				item.PutProperty("TextString", []interface{}{StrReplace(ts.ToString())}...)
			}
			item.Release()
		}
		slog.Debug(spaceName + " replaces end")
	}

	//Получаем листы
	slog.Debug("Получаем листы")
	layoutsv, err := activeDoc.GetProperty("Layouts")
	if err != nil {
		return err
	}
	layouts := layoutsv.ToIDispatch()
	defer layouts.Release()

	slog.Debug("Переключаемся на первый лист")
	itemv, err := layouts.CallMethod("Item", []interface{}{1}...)
	if err != nil {
		return err
	}

	_, err = activeDoc.PutProperty("ActiveLayout", []interface{}{itemv}...)
	if err != nil {
		return err
	}

	//Получаем конфигурации печати
	slog.Debug("Получаем конфигурации печати")
	pconfv, err := activeDoc.GetProperty("PlotConfigurations")
	if err != nil {
		return err
	}
	pconf := pconfv.ToIDispatch()
	defer pconf.Release()

	pcount, err := pconf.GetProperty("Count")
	if err != nil {
		return err
	}

	bgp, err := activeDoc.CallMethod("GetVariable", []interface{}{"BACKGROUNDPLOT"}...)
	if err != nil {
		return err
	}
	slog.Debug("BACKGROUNDPLOT is", slog.Int("value", int(bgp.Val)))

	//Устанавливаем BACKGROUNDPLOT в 0
	slog.Debug("Устанавливаем BACKGROUNDPLOT в 0")
	_, err = activeDoc.CallMethod("SetVariable", []interface{}{"BACKGROUNDPLOT", 0}...)
	if err != nil {
		return err
	}

	plotv, err := activeDoc.GetProperty("Plot")
	if err != nil {
		return err
	}
	plot := plotv.ToIDispatch()
	defer plot.Release()

	activeLayoutv, err := activeDoc.GetProperty("ActiveLayout")
	if err != nil {
		return err
	}
	activeLayout := activeLayoutv.ToIDispatch()
	defer activeLayout.Release()

	time.Sleep(time.Millisecond * 300)

	//Получаем список конфигураций и печатаем их
	slog.Debug("Получаем список конфигураций и печатаем их")
	for i := int32(0); i < pcount.Value().(int32); i++ {
		itemv, err := pconf.CallMethod("Item", []interface{}{i}...)
		if err != nil {
			return err
		}
		item := itemv.ToIDispatch()

		itemName, err := item.GetProperty("Name")
		if err != nil {
			item.Release()
			return err
		}
		slog.Debug(itemName.ToString())

		_, err = activeLayout.CallMethod("CopyFrom", []interface{}{item}...)
		time.Sleep(time.Millisecond * 300)
		item.Release()
		if err != nil {
			return err
		}

		plotArguments := []interface{}{filepath.Join(toDir, itemName.ToString()+".pdf")}
		slog.Debug("plotArguments " + filepath.Join(toDir, itemName.ToString()+".pdf"))

		oleutil.MustCallMethod(plot, "PlotToFile", plotArguments...)
		time.Sleep(time.Millisecond * 300)
	}

	//Устанавливаем BACKGROUNDPLOT обратно
		_, err = activeDoc.CallMethod("SetVariable", []interface{}{"BACKGROUNDPLOT", bgp.Value()}...)
	if err != nil {
		slog.Error(err.Error())
	}
	slog.Debug("BACKGROUNDPLOT restored", slog.Int("value", int(bgp.Value().(int16))))

	//Закрываем документ без сохранения
	slog.Debug("Закрываем документ без сохранения")
	_, err = activeDoc.CallMethod("Close", []interface{}{false}...)
	if err != nil {
		slog.Error(err.Error())
	}

	slog.Debug("Конец AcadToPdf")
	return nil
}

// A2pdf конвертирует DWG/DXF файлы в PDF через пул AutoCAD.
func A2pdf(ctx context.Context, sourceFile, sourceFolder, outputFolder string) error {
	var inputCadFiles []string

	//Если задан входной файл
	if sourceFile != "" {
		if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
			slog.ErrorContext(ctx, "Input file "+sourceFile+" does not exists")
			return err
		}

		ifn, err := filepath.Abs(sourceFile)
		if err != nil {
			slog.ErrorContext(ctx, err.Error())
			return err
		}
		inputCadFiles = append(inputCadFiles, ifn)
	}

	//Ищем в папке файлы
	if sourceFolder != "" {
		files, err := os.ReadDir(sourceFolder)
		if err != nil {
			slog.ErrorContext(ctx, err.Error())
			return err
		}

		for _, file := range files {
			if !file.IsDir() {
				ext := strings.ToLower(filepath.Ext(file.Name()))
				if ext == ".dwg" || ext == ".dxf" {
					ffn, err := filepath.Abs(filepath.Join(sourceFolder, file.Name()))
					if err != nil {
						slog.ErrorContext(ctx, err.Error())
						return err
					}
					inputCadFiles = append(inputCadFiles, ffn)
				}
			}
		}
	}

	if len(inputCadFiles) == 0 {
		slog.InfoContext(ctx, "Нет DWG/DXF файлов для конвертации")
		return nil
	}

	pool := NewAcadPool(1)
	defer pool.Close()

	var wg sync.WaitGroup
	for _, file := range inputCadFiles {
		wg.Add(1)
		go func(f string) {
			defer wg.Done()

			outDir, err := os.MkdirTemp("", "aconv-")
			if err != nil {
				slog.ErrorContext(ctx, err.Error())
				return
			}
			defer os.RemoveAll(outDir)

			err = pool.AcadToPdf(ctx, f, outDir)
			if err != nil {
				slog.ErrorContext(ctx, err.Error())
				return
			}

			cleanName := strings.TrimSuffix(f, filepath.Ext(f))
			slog.Debug("merge output", slog.String("file", filepath.Join(outputFolder, filepath.Base(cleanName)+".pdf")))

			err = pdf.Merge(ctx, outDir, filepath.Join(outputFolder, filepath.Base(cleanName)+".pdf"))
			if err != nil {
				slog.ErrorContext(ctx, err.Error())
			}
		}(file)
	}
	wg.Wait()

	return nil
}
