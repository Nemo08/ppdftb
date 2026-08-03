package slogutil

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"testing"
)

func TestSetupNoPanic(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error", "none", "unknown", "", "invalid_level"}
	for _, lvl := range levels {
		t.Run("level_"+lvl, func(t *testing.T) {
			Setup(lvl)
		})
	}
}

func TestSetupErrorLevels(t *testing.T) {
	Setup("error")

	h := slog.Default().Handler()
	if h.Enabled(context.TODO(), slog.LevelInfo) {
		t.Error("expected info to be disabled for 'error' level")
	}
	if h.Enabled(context.TODO(), slog.LevelWarn) {
		t.Error("expected warn to be disabled for 'error' level")
	}
	if !h.Enabled(context.TODO(), slog.LevelError) {
		t.Error("expected error to be enabled for 'error' level")
	}
}

func TestSetupUnknownLevelUsesError(t *testing.T) {
	Setup("unknown")

	h := slog.Default().Handler()
	if h.Enabled(context.TODO(), slog.LevelWarn) {
		t.Error("expected warn to be disabled for unknown level (defaults to error)")
	}
	if !h.Enabled(context.TODO(), slog.LevelError) {
		t.Error("expected error to be enabled for unknown level")
	}
}

func TestSetupDebugEnablesDebug(t *testing.T) {
	Setup("debug")

	h := slog.Default().Handler()
	if !h.Enabled(context.TODO(), slog.LevelDebug) {
		t.Error("expected debug to be enabled for 'debug' level")
	}
}

func TestSetupOutput(t *testing.T) {
	tests := []struct {
		level    string
		logFn    func()
		disabled bool // ожидается, что запись подавлена уровнем
	}{
		{"info", func() { slog.Info("test message info") }, false},
		{"error", func() { slog.Info("test message error") }, true},
		{"warn", func() { slog.Warn("test message warn") }, false},
		{"error", func() { slog.Warn("test message warn2") }, true},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = r.Close() }()

			old := os.Stdout
			os.Stdout = w

			Setup(tt.level)
			tt.logFn()

			_ = w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			_, _ = buf.ReadFrom(r)

			if tt.disabled && buf.Len() > 0 {
				t.Errorf("expected no output, got: %s", buf.String())
			}
			if !tt.disabled && buf.Len() == 0 {
				t.Error("expected output, got none")
			}
		})
	}
}
