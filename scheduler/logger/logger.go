// Package logger provides the shared loggers used throughout the scheduler.
package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"time"
)

var (
	level  = new(slog.LevelVar) // INFO by default, lowered to DEBUG by Init(true)
	opts   = &slog.HandlerOptions{AddSource: true, Level: level}
	stdout = slog.New(slog.NewTextHandler(os.Stdout, opts))
	stderr = slog.New(slog.NewTextHandler(os.Stderr, opts))
)

// Init configures the loggers. Call once from main. If debug is true, the level is
// lowered to DEBUG, which allows DebugLogger messages to be output.
func Init(debug bool) {
	if debug {
		level.Set(slog.LevelDebug)
	}
}

// InfoLogger logs an info level message to stdout with the given format and arguments
func InfoLogger(format string, args ...any) {
	logf(stdout, slog.LevelInfo, format, args...)
}

// WarnLogger logs a warning level message to stderr with the given format and arguments
func WarnLogger(format string, args ...any) {
	logf(stderr, slog.LevelWarn, format, args...)
}

// ErrorLogger logs an error level message to stderr with the given format and arguments
func ErrorLogger(format string, args ...any) {
	logf(stderr, slog.LevelError, format, args...)
}

// DebugLogger logs a debug level message to stdout with the given format and arguments.
// Messages are only output after Init(true) has been called.
func DebugLogger(format string, args ...any) {
	logf(stdout, slog.LevelDebug, format, args...)
}

// logf attributes the record to the wrapper's caller so AddSource reports the right file:line.
func logf(l *slog.Logger, level slog.Level, format string, args ...any) {
	ctx := context.Background()
	if !l.Enabled(ctx, level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:]) // skip runtime.Callers, logf and the exported wrapper
	r := slog.NewRecord(time.Now(), level, fmt.Sprintf(format, args...), pcs[0])
	_ = l.Handler().Handle(ctx, r)
}
