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

func TestSetupErrorDoesNotChangeLogger(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(h))

	Setup("error")

	if hh := slog.Default().Handler(); hh != h {
		t.Error("Setup('error') should not replace the handler")
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
		level string
		want  string
	}{
		{"info", "test message info"},
		{"error", ""},
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
			slog.Info("test message " + tt.level)

			_ = w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			_, _ = buf.ReadFrom(r)

			if tt.want == "" && buf.Len() > 0 {
				t.Errorf("expected no output, got: %s", buf.String())
			}
			if tt.want != "" && !bytes.Contains(buf.Bytes(), []byte(tt.want)) {
				t.Errorf("expected output containing %q, got: %s", tt.want, buf.String())
			}
		})
	}
}
