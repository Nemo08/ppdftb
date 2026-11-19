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
// Уровень берётся из командной строки и применяется всегда, включая "error".
func Setup(level string) {
	lvl, ok := levels[level]
	if !ok {
		lvl = slog.LevelError
	}
	opts := &slog.HandlerOptions{
		Level:     lvl,
		AddSource: true,
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, opts)))
}
