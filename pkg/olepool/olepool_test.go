//go:build windows

package olepool

import (
	"context"
	"errors"
	"testing"
	"time"

	ole "github.com/go-ole/go-ole"
)

type testJob struct {
	fn func(app *ole.IDispatch) error
}

func (j *testJob) Process(app *ole.IDispatch) error {
	return j.fn(app)
}

func TestNewPoolSizeZero(t *testing.T) {
	p := NewPool(0, Config{AppName: "Scripting.FileSystemObject"})
	if len(p.workers) != 0 {
		t.Fatalf("NewPool(0) = %d workers, want 0", len(p.workers))
	}
	p.Close()
}

func TestNewPoolSizeNegative(t *testing.T) {
	p := NewPool(-5, Config{AppName: "Scripting.FileSystemObject"})
	if len(p.workers) != 0 {
		t.Fatalf("NewPool(-5) = %d workers, want 0", len(p.workers))
	}
	p.Close()
}

func TestPoolSubmitNoWorkers(t *testing.T) {
	p := NewPool(0, Config{AppName: "Scripting.FileSystemObject"})
	defer p.Close()

	ctx := context.Background()
	err := p.Submit(ctx, &testJob{fn: func(app *ole.IDispatch) error {
		return nil
	}})
	if err == nil {
		t.Fatal("Submit with 0 workers should return error")
	}
}

func TestPoolCloseIdempotent(t *testing.T) {
	p := NewPool(1, Config{AppName: "Scripting.FileSystemObject"})
	p.Close()
	p.Close()
}

func TestPoolSubmitSuccess(t *testing.T) {
	p := NewPool(1, Config{AppName: "Scripting.FileSystemObject"})
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := p.Submit(ctx, &testJob{fn: func(app *ole.IDispatch) error {
		return nil
	}})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
}

func TestPoolSubmitContextCancel(t *testing.T) {
	p := NewPool(1, Config{AppName: "Scripting.FileSystemObject"})
	defer p.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.Submit(ctx, &testJob{fn: func(app *ole.IDispatch) error {
		return nil
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Submit with cancelled ctx = %v, want context.Canceled", err)
	}
}

// TestPoolSubmitCloseRace проверяет, что одновременные вызовы Submit и Close
// не вызывают panic (send on closed channel).
func TestPoolSubmitCloseRace(t *testing.T) {
	p := NewPool(4, Config{AppName: "Scripting.FileSystemObject"})

	done := make(chan struct{})
	go func() {
		for range 100 {
			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
			_ = p.Submit(ctx, &testJob{fn: func(app *ole.IDispatch) error {
				return nil
			}})
			cancel()
		}
		close(done)
	}()

	p.Close()
	<-done
}

// TestPoolSubmitAfterClose проверяет, что Submit после Close возвращает ошибку.
func TestPoolSubmitAfterClose(t *testing.T) {
	p := NewPool(1, Config{AppName: "Scripting.FileSystemObject"})
	p.Close()

	ctx := context.Background()
	err := p.Submit(ctx, &testJob{fn: func(app *ole.IDispatch) error {
		return nil
	}})
	if err == nil {
		t.Fatal("Submit after Close should return error")
	}
}
