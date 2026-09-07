//go:build windows

// Package olepool — общий пул COM-приложений (Word/AutoCAD) с управлением
// жизненным циклом воркеров, Job Object для завершения зависших процессов
// и ожиданием готовности.
package olepool

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"log/slog"

	"github.com/Nemo08/ppdftb/pkg/jobutil"
	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
)

// Job — задание для пула OLE-объектов.
type Job interface {
	Process(app *ole.IDispatch) error
}

type jobWrap struct {
	job    Job
	result chan error
}

// Config — настройка пула.
type Config struct {
	AppName string
	Setup   func(app *ole.IDispatch)
}

// Pool — обобщённый пул OLE-объектов (Word, AutoCAD и т.д.).
type Pool struct {
	workers   []*worker
	jobs      chan jobWrap
	once      sync.Once
	mu        sync.Mutex
	closed    bool
	jobHandle windows.Handle

	// ready закрывается когда хотя бы один воркер успешно инициализировался.
	ready     chan struct{}
	readyOnce sync.Once
}

type worker struct {
	pool *Pool
	done chan struct{}
	// pids — процессы, поднятые этим воркером (Hwnd, либо fallback-снимок до/после CreateObject).
	pids []uint32
}

var (
	modUser32                    = windows.NewLazySystemDLL("user32.dll")
	procGetWindowThreadProcessId = modUser32.NewProc("GetWindowThreadProcessId")
)

// NewPool создаёт пул из size экземпляров COM-приложения.
// Каждый экземпляр работает в своём OS-потоке (требование COM).
// Если size <= 0, пул создаётся без воркеров — Submit сразу вернёт ошибку.
func NewPool(size int, cfg Config) *Pool {
	if size <= 0 {
		slog.Debug("OlePool не запущен (size <= 0)", slog.String("app", cfg.AppName))
		p := &Pool{ready: make(chan struct{})}
		close(p.ready)
		return p
	}
	p := &Pool{
		jobs:      make(chan jobWrap, size*4),
		workers:   make([]*worker, size),
		jobHandle: jobutil.CreateJobObject(),
		ready:     make(chan struct{}),
	}
	for i := range p.workers {
		w := &worker{pool: p, done: make(chan struct{})}
		p.workers[i] = w
		go w.run(cfg)
	}
	slog.Debug("OlePool запущен", slog.Int("workers", size), slog.String("app", cfg.AppName))
	return p
}

// WaitReady блокируется пока хотя бы один воркер не будет готов принимать задания.
func (p *Pool) WaitReady(ctx context.Context) error {
	select {
	case <-p.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Submit отправляет задание в пул и ждёт результат.
func (p *Pool) Submit(ctx context.Context, job Job) error {
	if len(p.workers) == 0 {
		return errors.New("пул не содержит воркеров")
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return errors.New("пул закрыт")
	}
	result := make(chan error, 1)
	select {
	case p.jobs <- jobWrap{job: job, result: result}:
		p.mu.Unlock()
	case <-ctx.Done():
		p.mu.Unlock()
		return ctx.Err()
	}
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close завершает все экземпляры и освобождает ресурсы.
// Прибивает только PID, привязанные воркерами (свои).
func (p *Pool) Close() {
	if len(p.workers) == 0 {
		return
	}
	p.once.Do(func() {
		p.mu.Lock()
		p.closed = true
		close(p.jobs)
		p.mu.Unlock()
		for _, w := range p.workers {
			<-w.done
		}
		for _, w := range p.workers {
			if len(w.pids) > 0 {
				jobutil.KillProcesses(w.pids)
			}
		}
		if p.jobHandle != 0 {
			// KILL_ON_JOB_CLOSE — добить детей, если остались в job.
			if err := windows.CloseHandle(p.jobHandle); err != nil {
				slog.Warn("не удалось закрыть Job Object", slog.String("err", err.Error()))
			}
			p.jobHandle = 0
		}
		slog.Debug("OlePool остановлен")
	})
}

func (w *worker) run(cfg Config) {
	defer close(w.done)

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := initCOM(); err != nil {
		w.signalInitFailed("CoInitializeEx", err)
		return
	}
	defer ole.CoUninitialize()

	// Снимок до CreateObject — только для fallback, если Hwnd не отдаст PID.
	var beforePIDs []uint32
	if w.pool.jobHandle != 0 {
		beforePIDs = jobutil.GetAllPids()
	}

	app, err := createApp(cfg.AppName)
	if err != nil {
		w.signalInitFailed(cfg.AppName, err)
		return
	}
	defer app.Release()

	w.bindOwnedProcess(app, beforePIDs)

	if cfg.Setup != nil {
		cfg.Setup(app)
	}

	w.pool.readyOnce.Do(func() { close(w.pool.ready) })
	slog.Debug("OLE воркер готов",
		slog.String("app", cfg.AppName),
		slog.Any("pids", w.pids))

	for wrap := range w.pool.jobs {
		func(j Job, result chan error) {
			defer func() {
				if r := recover(); r != nil {
					err := fmt.Errorf("panic в OLE-задании: %v", r)
					slog.Error("OLE воркер", slog.String("err", err.Error()))
					result <- err
				}
			}()
			result <- j.Process(app)
		}(wrap.job, wrap.result)
	}

	if _, err := oleutil.CallMethod(app, "Quit"); err != nil {
		slog.Debug("Quit", slog.String("app", cfg.AppName), slog.String("err", err.Error()))
	}
}

// bindOwnedProcess привязывает процесс, поднятый этим воркером.
//
//  1. Основной путь: Application.Hwnd → GetWindowThreadProcessId → один PID.
//  2. Fallback: новые PID между before и after CreateObject (старый снимок),
//     чтобы при Hwnd=0 / отсутствии свойства всё равно убить то, что подняли,
//     и повесить на Job Object (KILL_ON_JOB_CLOSE).
func (w *worker) bindOwnedProcess(app *ole.IDispatch, beforePIDs []uint32) {
	if pid, err := processIDFromApp(app); err == nil {
		w.pids = []uint32{pid}
		w.assignToJob(w.pids)
		slog.Debug("COM-процесс привязан по Hwnd", slog.Uint64("pid", uint64(pid)))
		return
	} else {
		slog.Warn("Hwnd→PID недоступен, fallback на снимок PID",
			slog.String("err", err.Error()))
	}

	if w.pool.jobHandle == 0 {
		return
	}
	afterPIDs := jobutil.GetAllPids()
	w.pids = jobutil.CollectNewPids(beforePIDs, afterPIDs)
	w.assignToJob(w.pids)
	slog.Debug("COM-процессы привязаны fallback-снимком", slog.Any("pids", w.pids))
}

func (w *worker) assignToJob(pids []uint32) {
	if w.pool.jobHandle == 0 || len(pids) == 0 {
		return
	}
	// before=nil → в job попадают все переданные pid
	jobutil.AssignPidsToJob(w.pool.jobHandle, nil, pids)
}

// processIDFromApp читает Hwnd/HWND у Application и резолвит PID через user32.
func processIDFromApp(app *ole.IDispatch) (uint32, error) {
	var hwnd uintptr
	for _, prop := range []string{"Hwnd", "HWND", "hwnd"} {
		v, err := oleutil.GetProperty(app, prop)
		if err != nil {
			continue
		}
		hwnd = uintptr(v.Val)
		v.Clear()
		if hwnd != 0 {
			break
		}
	}
	if hwnd == 0 {
		return 0, errors.New("свойство Hwnd/HWND отсутствует или равно 0")
	}
	return pidFromHwnd(hwnd)
}

func pidFromHwnd(hwnd uintptr) (uint32, error) {
	var pid uint32
	r1, _, err := procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if r1 == 0 {
		if err != nil {
			return 0, err
		}
		return 0, errors.New("GetWindowThreadProcessId вернул 0")
	}
	if pid == 0 {
		return 0, errors.New("PID = 0")
	}
	return pid, nil
}

func (w *worker) signalInitFailed(context string, err error) {
	slog.Error(context, slog.String("err", err.Error()))
	w.pool.readyOnce.Do(func() { close(w.pool.ready) })
}

func initCOM() error {
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		oleErr := &ole.OleError{}
		if errors.As(err, &oleErr) {
			return err
		}
	}
	return nil
}

func createApp(appName string) (*ole.IDispatch, error) {
	unknown, err := oleutil.CreateObject(appName)
	if err != nil {
		return nil, err
	}
	app, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		unknown.Release()
		return nil, err
	}
	unknown.Release()
	return app, nil
}
