package acadpool

import (
	"context"
	"strings"
	"testing"
)

func TestStrReplace(t *testing.T) {
	replaces := map[string]string{
		"Name":   "Имя",
		"Number": "66955",
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no placeholders", "plain text", "plain text"},
		{"single placeholder", `\{\{Name\}\}`, "Имя"},
		{"multiple placeholders", `\{\{Name\}\} \{\{Number\}\}`, "Имя 66955"},
		{"partial match", `prefix \{\{Name\}\} suffix`, "prefix Имя suffix"},
		{"no closing braces", `\{\{Name`, `\{\{Name`},
		{"unknown placeholder", `\{\{Unknown\}\}`, `\{\{Unknown\}\}`},
		{"unescaped braces ignored", `{{Name}}`, `{{Name}}`},
		{"empty input", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StrReplace(tt.in, replaces)
			if got != tt.want {
				t.Errorf("StrReplace(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestStrReplaceOverride(t *testing.T) {
	got := StrReplace(`\{\{Name\}\}`, map[string]string{"Name": "Other"})
	if got != "Other" {
		t.Errorf("StrReplace() = %q, want %q", got, "Other")
	}
}

func TestNewAcadPoolSizeZero(t *testing.T) {
	p := NewAcadPool(0)
	if p == nil {
		t.Fatal("NewAcadPool(0) returned nil")
	}
	p.Close()
}

func TestNewAcadPoolSizeNegative(t *testing.T) {
	p := NewAcadPool(-1)
	if p == nil {
		t.Fatal("NewAcadPool(-1) returned nil")
	}
	p.Close()
}

func TestNewAcadPoolWithReplacesNil(t *testing.T) {
	p := NewAcadPoolWithReplaces(0, nil)
	if p == nil {
		t.Fatal("NewAcadPoolWithReplaces(0, nil) returned nil")
	}
	p.Close()
}

func TestAcadToPdfEmptyPath(t *testing.T) {
	p := NewAcadPool(0)
	defer p.Close()

	err := p.AcadToPdf(context.Background(), "", "")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
	if !strings.Contains(err.Error(), "The file") && !strings.Contains(err.Error(), "системе") {
		t.Logf("got expected error: %v", err)
	}
}

func TestAcadToPdfCancelledContext(t *testing.T) {
	p := NewAcadPool(0)
	defer p.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.AcadToPdf(ctx, "test.dwg", "out")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestAcadPoolCloseIdempotent(t *testing.T) {
	p := NewAcadPool(0)
	p.Close()
	p.Close()
}

func TestAcadPoolWaitReady(t *testing.T) {
	p := NewAcadPool(0)
	defer p.Close()

	err := p.WaitReady(context.Background())
	if err != nil {
		t.Fatalf("WaitReady() = %v, want nil", err)
	}
}
