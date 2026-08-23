package logger

import (
	"fmt"
	"log/slog"
	"os"
)

// InfoLogger logs an info level message with the given format and arguments
func InfoLogger(format string, args ...any) {
	slog.Info(fmt.Sprintf(format, args...))
}

// WarnLogger logs a warning level message with the given format and arguments
func WarnLogger(format string, args ...any) {
	slog.Warn(fmt.Sprintf(format, args...))
}

// ErrorLogger logs an error level message with the given format and arguments
func ErrorLogger(format string, args ...any) {
	slog.Error(fmt.Sprintf(format, args...))
}

// DebugLogger logs a debug level message with the given format and arguments
func DebugLogger(format string, args ...any) {
	slog.Debug(fmt.Sprintf(format, args...))
}

// FatalErrorLogger logs an error level message and exits with status code 1
func FatalErrorLogger(format string, args ...any) {
	slog.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}
