//go:build windows

package olepool

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/Nemo08/ppdftb/pkg/jobutil"
	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
	"log/slog"
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
	jobHandle windows.Handle
}

type worker struct {
	pool *Pool
	done chan struct{}
	pids []uint32
}

// NewPool создаёт пул из size экземпляров COM-приложения.
// Каждый экземпляр работает в своём OS-потоке (требование COM).
// Если size <= 0, пул создаётся без воркеров — Submit сразу вернёт ошибку.
func NewPool(size int, cfg Config) *Pool {
	if size <= 0 {
		slog.Debug("OlePool не запущен (size <= 0)", slog.String("app", cfg.AppName))
		return &Pool{}
	}
	p := &Pool{
		jobs:      make(chan jobWrap, size*4),
		workers:   make([]*worker, size),
		jobHandle: jobutil.CreateJobObject(),
	}
	for i := range p.workers {
		w := &worker{pool: p, done: make(chan struct{})}
		p.workers[i] = w
		go w.run(cfg)
	}
	slog.Debug("OlePool запущен", slog.Int("workers", size), slog.String("app", cfg.AppName))
	return p
}

// Submit отправляет задание в пул и ждёт результат.
func (p *Pool) Submit(ctx context.Context, job Job) error {
	if len(p.workers) == 0 {
		return fmt.Errorf("пул не содержит воркеров")
	}
	result := make(chan error, 1)
	select {
	case p.jobs <- jobWrap{job: job, result: result}:
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

// Close завершает все экземпляры и освобождает ресурсы.
func (p *Pool) Close() {
	if len(p.workers) == 0 {
		return
	}
	p.once.Do(func() {
		close(p.jobs)
		for _, w := range p.workers {
			<-w.done
		}
		for _, w := range p.workers {
			jobutil.KillProcesses(w.pids)
		}
		if p.jobHandle != 0 {
			windows.CloseHandle(p.jobHandle)
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
		w.failAll("CoInitializeEx", err)
		return
	}
	defer ole.CoUninitialize()

	beforePIDs := w.snapshotPIDs()

	app, err := createApp(cfg.AppName)
	if err != nil {
		w.failAll(cfg.AppName, err)
		return
	}
	defer app.Release()

	w.capturePIDs(beforePIDs)

	if cfg.Setup != nil {
		cfg.Setup(app)
	}

	slog.Debug("OLE воркер готов", slog.String("app", cfg.AppName))

	for wrap := range w.pool.jobs {
		wrap.result <- wrap.job.Process(app)
	}

	if _, err := oleutil.CallMethod(app, "Quit"); err != nil {
		slog.Debug("Quit", slog.String("app", cfg.AppName), slog.String("err", err.Error()))
	}
}

func initCOM() error {
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		if oleErr, ok := err.(*ole.OleError); !ok || oleErr.Code() != 0x00000001 {
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

func (w *worker) failAll(context string, err error) {
	slog.Error(context, slog.String("err", err.Error()))
	for wrap := range w.pool.jobs {
		wrap.result <- fmt.Errorf("%s: %w", context, err)
	}
}

func (w *worker) snapshotPIDs() []uint32 {
	if w.pool.jobHandle == 0 {
		return nil
	}
	return jobutil.GetAllPids()
}

func (w *worker) capturePIDs(beforePIDs []uint32) {
	if w.pool.jobHandle == 0 {
		return
	}
	afterPIDs := jobutil.GetAllPids()
	jobutil.AssignPidsToJob(w.pool.jobHandle, beforePIDs, afterPIDs)
	w.pids = jobutil.CollectNewPids(beforePIDs, afterPIDs)
}
