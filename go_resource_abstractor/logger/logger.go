// Package logger configures the process-wide structured logger (log/slog)
// used throughout the resource abstractor. It reads the same LOG_LEVEL env
// var as the Python service (resource_abstractor.py), defaulting to DEBUG.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// Init configures slog's default logger to write text-formatted logs to
// stdout at the given level. levelName is matched case-insensitively against
// DEBUG, INFO, WARN/WARNING and ERROR; anything else falls back to DEBUG to
// match the Python service's default.
func Init(levelName string) {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(levelName),
	})
	slog.SetDefault(slog.New(handler))
}

func parseLevel(levelName string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(levelName)) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelDebug
	}
}
