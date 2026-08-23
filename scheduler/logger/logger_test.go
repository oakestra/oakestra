package logger

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestDebugMode verifies that DebugLogger respects the debug level setting
func TestDebugMode(t *testing.T) {
	// Save original default logger
	original := slog.Default()
	defer slog.SetDefault(original)

	// Test 1: At INFO level, DebugLogger should not output
	var buf1 bytes.Buffer
	handler1 := slog.NewTextHandler(&buf1, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger1 := slog.New(handler1)

	slog.SetDefault(logger1)

	DebugLogger("should not appear")

	if strings.Contains(buf1.String(), "should not appear") {
		t.Error("DebugLogger should not output at INFO level")
	}

	// Test 2: At DEBUG level, DebugLogger should output
	var buf2 bytes.Buffer
	handler2 := slog.NewTextHandler(&buf2, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger2 := slog.New(handler2)

	slog.SetDefault(logger2)
	DebugLogger("should appear")

	if !strings.Contains(buf2.String(), "should appear") {
		t.Error("DebugLogger should output at DEBUG level")
	}
}

// TestSetDebugMode verifies that SetDebugMode works without panicking
func TestSetDebugMode(t *testing.T) {
	// Save original default logger
	original := slog.Default()
	defer slog.SetDefault(original)

	// Set up a logger at INFO level
	var buf1 bytes.Buffer
	handler1 := slog.NewTextHandler(&buf1, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger1 := slog.New(handler1)
	slog.SetDefault(logger1)

	// Debug should not appear at INFO level
	DebugLogger("before SetDebugMode")
	if strings.Contains(buf1.String(), "before SetDebugMode") {
		t.Error("DebugLogger should not output before SetDebugMode")
	}

	// Call SetDebugMode - this creates a new default logger with DEBUG level
	// Note: SetDebugMode writes to os.Stdout, so we can't capture it in a buffer
	// But we can verify it doesn't panic
	SetDebugMode()

	// Verify SetDebugMode completed without panic (we got here)
	// The actual debug output goes to os.Stdout which is expected

	// Restore
	slog.SetDefault(original)
}

// TestLoggerFunctions tests that all logger functions work without panicking
func TestLoggerFunctions(t *testing.T) {
	// Save original default logger
	original := slog.Default()
	defer slog.SetDefault(original)

	// Create a test logger that writes to our buffer
	var buf bytes.Buffer
	testHandler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	testLogger := slog.New(testHandler)

	// Temporarily replace the default logger
	slog.SetDefault(testLogger)

	// Test all functions - they should not panic
	InfoLogger("info test: %s", "arg1")
	WarnLogger("warn test: %s", "arg2")
	ErrorLogger("error test: %s", "arg3")
	DebugLogger("debug test: %s", "arg4")

	// Verify output contains our messages
	output := buf.String()
	if !strings.Contains(output, "info test: arg1") {
		t.Errorf("Expected info message in output, got: %s", output)
	}
	if !strings.Contains(output, "warn test: arg2") {
		t.Errorf("Expected warn message in output, got: %s", output)
	}
	if !strings.Contains(output, "error test: arg3") {
		t.Errorf("Expected error message in output, got: %s", output)
	}
	if !strings.Contains(output, "debug test: arg4") {
		t.Errorf("Expected debug message in output, got: %s", output)
	}
}
