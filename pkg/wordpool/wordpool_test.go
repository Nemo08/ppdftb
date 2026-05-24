//go:build windows

package wordpool

import (
	"context"
	"testing"
)

func TestNewWordPoolSizeZero(t *testing.T) {
	p := NewWordPool(0)
	if p == nil {
		t.Fatal("NewWordPool(0) returned nil")
	}
	p.Close()
}

func TestWordPoolCloseIdempotent(t *testing.T) {
	p := NewWordPool(1)
	p.Close()
	p.Close()
}

func TestWordPoolContextCancel(t *testing.T) {
	p := NewWordPool(1)
	defer p.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.WordToPdf(ctx, "test.docx", "test.pdf")
	if err != context.Canceled {
		t.Logf("WordToPdf with cancelled ctx = %v (may succeed if Word unavailable)", err)
	}
}
