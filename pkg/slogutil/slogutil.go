// Package slogutil предоставляет единую настройку логгера для всех CLI-утилит.
package slogutil

import (
	"log/slog"
	"os"
)

var levels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
	"none":  -8,
}

// Setup настраивает глобальный логгер slog на указанный уровень.
// Если level == "error", логгер не меняется (дефолтный JSONHandler).
func Setup(level string) {
	if level == "error" {
		return
	}
	opts := &slog.HandlerOptions{
		Level:     levels[level],
		AddSource: true,
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, opts)))
}
