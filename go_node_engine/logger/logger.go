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
	opts   = &slog.HandlerOptions{AddSource: true}
	stdout = slog.New(slog.NewTextHandler(os.Stdout, opts))
	stderr = slog.New(slog.NewTextHandler(os.Stderr, opts))
)

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

// DebugLogger logs a debug level message to stdout with the given format and arguments
func DebugLogger(format string, args ...any) {
	logf(stdout, slog.LevelDebug, format, args...)
}

// FatalErrorLogger logs an error level message to stderr and exits with status code 1
func FatalErrorLogger(format string, args ...any) {
	logf(stderr, slog.LevelError, format, args...)
	os.Exit(1)
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
