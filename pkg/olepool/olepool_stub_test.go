// Тесты olepool без реального COM — проверяют логику ready/submit/close
// на stub-реализации, которая не требует Windows.
// Сборка без тега windows — работает на любой платформе.

package olepool_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- минимальный stub пула (повторяет логику olepool.Pool без COM) ---

type stubJob struct {
	fn func() error
}

type stubWrap struct {
	job    *stubJob
	result chan error
}

type stubPool struct {
	jobs      chan stubWrap
	workers   int
	once      sync.Once
	mu        sync.Mutex
	closed    bool
	ready     chan struct{}
	readyOnce sync.Once
	workerWg  sync.WaitGroup
}

func newStubPool(size int, initDelay time.Duration) *stubPool {
	p := &stubPool{
		jobs:    make(chan stubWrap, size*4),
		workers: size,
		ready:   make(chan struct{}),
	}
	for i := 0; i < size; i++ {
		p.workerWg.Add(1)
		go p.runWorker(initDelay)
	}
	return p
}

func (p *stubPool) runWorker(initDelay time.Duration) {
	defer p.workerWg.Done()
	time.Sleep(initDelay) // симулируем инициализацию COM
	p.readyOnce.Do(func() { close(p.ready) })
	for wrap := range p.jobs {
		wrap.result <- wrap.job.fn()
	}
}

func (p *stubPool) waitReady(ctx context.Context) error {
	select {
	case <-p.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *stubPool) submit(ctx context.Context, fn func() error) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return errors.New("пул закрыт")
	}
	result := make(chan error, 1)
	select {
	case p.jobs <- stubWrap{job: &stubJob{fn: fn}, result: result}:
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

func (p *stubPool) close() {
	p.once.Do(func() {
		p.mu.Lock()
		p.closed = true
		close(p.jobs)
		p.mu.Unlock()
		p.workerWg.Wait()
	})
}

// --- тесты ---

func TestStubPool_ReadyBeforeSubmit(t *testing.T) {
	p := newStubPool(2, 50*time.Millisecond)
	defer p.close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := p.waitReady(ctx); err != nil {
		t.Fatalf("WaitReady вернул ошибку: %v", err)
	}

	err := p.submit(ctx, func() error { return nil })
	if err != nil {
		t.Fatalf("submit вернул ошибку: %v", err)
	}
}

func TestStubPool_SubmitWithoutWaitReady(t *testing.T) {
	// submit должен работать даже если WaitReady не вызывался —
	// он просто заблокируется пока воркер не стартует.
	p := newStubPool(1, 100*time.Millisecond)
	defer p.close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var called bool
	err := p.submit(ctx, func() error { called = true; return nil })
	if err != nil {
		t.Fatalf("submit вернул ошибку: %v", err)
	}
	if !called {
		t.Error("job не был вызван")
	}
}

func TestStubPool_WaitReadyTimeout(t *testing.T) {
	// initDelay > timeout → WaitReady должен вернуть context.DeadlineExceeded.
	p := newStubPool(1, 500*time.Millisecond)
	defer p.close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := p.waitReady(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ожидали DeadlineExceeded, получили: %v", err)
	}
}

func TestStubPool_Parallel(t *testing.T) {
	const numWorkers = 4
	const numJobs = 20
	p := newStubPool(numWorkers, 0)
	defer p.close()

	ctx := context.Background()
	if err := p.waitReady(ctx); err != nil {
		t.Fatal(err)
	}

	var counter atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < numJobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.submit(ctx, func() error {
				counter.Add(1)
				return nil
			})
		}()
	}
	wg.Wait()

	if got := counter.Load(); got != numJobs {
		t.Errorf("ожидали %d выполненных заданий, получили %d", numJobs, got)
	}
}

func TestStubPool_SubmitAfterClose(t *testing.T) {
	p := newStubPool(1, 0)
	ctx := context.Background()
	if err := p.waitReady(ctx); err != nil {
		t.Fatal(err)
	}
	p.close()

	err := p.submit(ctx, func() error { return nil })
	if err == nil {
		t.Error("ожидали ошибку при submit после close")
	}
}

func TestStubPool_ContextCancelDuringSubmit(t *testing.T) {
	// Пул без воркеров — задание никогда не будет обработано.
	p := &stubPool{
		jobs:  make(chan stubWrap, 0), // буфер 0 — submit заблокируется
		ready: make(chan struct{}),
	}
	close(p.ready)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := p.submit(ctx, func() error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ожидали Canceled, получили: %v", err)
	}
}

func TestStubPool_ErrorPropagation(t *testing.T) {
	p := newStubPool(1, 0)
	defer p.close()

	ctx := context.Background()
	if err := p.waitReady(ctx); err != nil {
		t.Fatal(err)
	}

	sentinel := errors.New("sentinel error")
	err := p.submit(ctx, func() error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Errorf("ожидали sentinel error, получили: %v", err)
	}
}
