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
	"strings"
	"sync"
	"time"
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
	// ExeNames — имена исполняемых файлов (например "WINWORD.EXE", "ACAD.EXE"),
	// по которым в fallback (когда Hwnd не отдал PID) отыскивается процесс,
	// поднятый данным воркером. Фильтр по имени не даёт захватить чужие или
	// соседние процессы, стартовавшие в то же окно.
	ExeNames []string
	Setup    func(app *ole.IDispatch)
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

// Параметры ожидания Hwnd: главное окно COM-приложения может создаваться
// асинхронно после CreateObject/Setup, поэтому Hwnd читаем с паузами.
const (
	hwndPollInterval = 100 * time.Millisecond
	hwndPollTimeout  = 3 * time.Second
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

	// Снимок СВОИХ процессов (по имени exe) до CreateObject. Fallback считает
	// только такие разности — чтобы не захватить уже запущенный пользователем
	// Word и не трогать чужие/соседние PID.
	beforeExe := jobutil.FindProcessesByName(cfg.ExeNames...)

	app, err := createApp(cfg.AppName)
	if err != nil {
		w.signalInitFailed(cfg.AppName, err)
		return
	}
	defer app.Release()

	if cfg.Setup != nil {
		cfg.Setup(app)
	}

	// Привязку процесса делаем ПОСЛЕ Setup: к этому моменту применён
	// Visible=false, главное окно создано, и Hwnd уже не равен 0.
	w.bindOwnedProcess(app, cfg.ExeNames, beforeExe)

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
//     Чтение Hwnd делается с ретраем: окно COM-приложения может создаваться
//     асинхронно и сразу после CreateObject/Setup ещё быть равным 0.
//  2. Fallback: среди процессов с именем из ExeNames берём только те, что
//     появились ПОСЛЕ CreateObject (beforeExe — снимок до него). Так не
//     захватываются ни уже запущенный пользователем Word, ни соседние PID.
func (w *worker) bindOwnedProcess(app *ole.IDispatch, exeNames []string, beforeExe []uint32) {
	if pid, err := processIDFromApp(app); err == nil {
		w.pids = []uint32{pid}
		w.assignToJob(w.pids)
		slog.Debug("COM-процесс привязан по Hwnd", slog.Uint64("pid", uint64(pid)))
		return
	} else {
		slog.Warn("Hwnd→PID недоступен, fallback на поиск по имени exe",
			slog.String("err", err.Error()))
	}

	if w.pool.jobHandle == 0 || len(exeNames) == 0 {
		return
	}
	afterExe := jobutil.FindProcessesByName(exeNames...)
	w.pids = jobutil.CollectNewPids(beforeExe, afterExe)
	w.assignToJob(w.pids)
	slog.Debug("COM-процессы привязаны fallback-поиском по имени",
		slog.String("exe", strings.Join(exeNames, ",")),
		slog.Any("pids", w.pids))
}

func (w *worker) assignToJob(pids []uint32) {
	if w.pool.jobHandle == 0 || len(pids) == 0 {
		return
	}
	// before=nil → в job попадают все переданные pid
	jobutil.AssignPidsToJob(w.pool.jobHandle, nil, pids)
}

// processIDFromApp читает Hwnd/HWND у Application и резолвит PID через user32.
// Чтение Hwnd повторяется с паузой: сразу после создания окно может быть
// ещё не готово (Hwnd = 0), что мы и наблюдаем у Word при Visible=false.
func processIDFromApp(app *ole.IDispatch) (uint32, error) {
	deadline := time.Now().Add(hwndPollTimeout)
	for {
		hwnd, err := readAppHwnd(app)
		if err == nil && hwnd != 0 {
			return pidFromHwnd(hwnd)
		}
		if time.Now().After(deadline) {
			if err != nil {
				return 0, err
			}
			return 0, errors.New("Hwnd/HWND остался равным 0 по таймауту")
		}
		time.Sleep(hwndPollInterval)
	}
}

// readAppHwnd достаёт Hwnd/HWND из Application, пробуя варианты имени свойства.
func readAppHwnd(app *ole.IDispatch) (uintptr, error) {
	for _, prop := range []string{"Hwnd", "HWND", "hwnd"} {
		v, err := oleutil.GetProperty(app, prop)
		if err != nil {
			continue
		}
		hwnd := uintptr(v.Val)
		v.Clear()
		if hwnd != 0 {
			return hwnd, nil
		}
	}
	return 0, errors.New("свойство Hwnd/HWND отсутствует или равно 0")
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
