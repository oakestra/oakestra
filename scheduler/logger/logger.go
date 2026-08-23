// Package logger provides the shared loggers used throughout the scheduler.
package logger

import (
	"fmt"
	"log/slog"
	"os"
)

// Init configures the default slog logger. Call once from main. If debug is true,
// the level is set to DEBUG, which allows DebugLogger messages to be output.
func Init(debug bool) {
	if debug {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
	}
}

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

// DebugLogger logs a debug level message with the given format and arguments.
// Messages are only output if the slog default logger's level is set to DEBUG
func DebugLogger(format string, args ...any) {
	slog.Debug(fmt.Sprintf(format, args...))
}
