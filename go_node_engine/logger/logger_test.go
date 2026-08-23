package logger

import (
	"bytes"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestFatalErrorLogger_LogsAndExits verifies that FatalErrorLogger
// logs an error message and exits with status code 1.
func TestFatalErrorLogger_LogsAndExits(t *testing.T) {
	// FatalErrorLogger calls os.Exit(1), so we need to test it in a subprocess
	if os.Getenv("TEST_FATAL") == "1" {
		// This is the subprocess - call FatalErrorLogger and exit
		FatalErrorLogger("test fatal message: %s", "arg")
		return
	}

	// Run in a subprocess to capture exit code
	cmd := exec.Command(os.Args[0], "-test.run=TestFatalErrorLogger_LogsAndExits")
	cmd.Env = append(os.Environ(), "TEST_FATAL=1")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Verify exit code is 1
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("Expected exit error, got %v", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("Expected exit code 1, got %d", exitErr.ExitCode())
	}

	// Verify error message was logged
	output := stderr.String()
	if !strings.Contains(output, "test fatal message: arg") {
		t.Errorf("Expected log message in stderr, got: %s", output)
	}
}

// TestLoggerFunctions tests that all logger functions work without panicking
func TestLoggerFunctions(t *testing.T) {
	// Capture output to verify it works
	var buf bytes.Buffer
	original := slog.Default().Handler()

	// Create a test logger that writes to our buffer
	testHandler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	testLogger := slog.New(testHandler)

	// Temporarily replace the default logger
	// Note: This affects global state, but it's okay for tests
	// We'll restore it after
	slog.SetDefault(testLogger)
	defer slog.SetDefault(slog.New(original))

	// Test all functions - they should not panic
	InfoLogger("info test: %s", "arg1")
	WarnLogger("warn test: %s", "arg2")
	ErrorLogger("error test: %s", "arg3")
	DebugLogger("debug test: %s", "arg4")

	// Verify output contains our messages
	output := buf.String()
	if !strings.Contains(output, "info test: arg1") {
		t.Errorf("Expected info message in output")
	}
	if !strings.Contains(output, "warn test: arg2") {
		t.Errorf("Expected warn message in output")
	}
	if !strings.Contains(output, "error test: arg3") {
		t.Errorf("Expected error message in output")
	}
	if !strings.Contains(output, "debug test: arg4") {
		t.Errorf("Expected debug message in output")
	}
}
