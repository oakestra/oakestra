package logger

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// captureOutput redirects the stdout and stderr loggers into buffers for the duration of the test.
func captureOutput(t *testing.T, level slog.Level) (out, errOut *bytes.Buffer) {
	t.Helper()
	origStdout, origStderr := stdout, stderr
	t.Cleanup(func() { stdout, stderr = origStdout, origStderr })

	out, errOut = &bytes.Buffer{}, &bytes.Buffer{}
	testOpts := &slog.HandlerOptions{AddSource: true, Level: level}
	stdout = slog.New(slog.NewTextHandler(out, testOpts))
	stderr = slog.New(slog.NewTextHandler(errOut, testOpts))
	return out, errOut
}

// TestFatalErrorLogger_LogsAndExits verifies that FatalErrorLogger
// logs an error message to stderr and exits with status code 1.
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

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

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
	output := stderrBuf.String()
	if !strings.Contains(output, "test fatal message: arg") {
		t.Errorf("Expected log message in stderr, got: %s", output)
	}
}

// TestLoggerFunctions verifies that INFO/DEBUG go to stdout and WARN/ERROR go to stderr.
func TestLoggerFunctions(t *testing.T) {
	out, errOut := captureOutput(t, slog.LevelDebug)

	InfoLogger("info test: %s", "arg1")
	DebugLogger("debug test: %s", "arg2")
	WarnLogger("warn test: %s", "arg3")
	ErrorLogger("error test: %s", "arg4")

	for _, msg := range []string{"info test: arg1", "debug test: arg2"} {
		if !strings.Contains(out.String(), msg) || strings.Contains(errOut.String(), msg) {
			t.Errorf("Expected %q only in stdout, got stdout=%q stderr=%q", msg, out, errOut)
		}
	}
	for _, msg := range []string{"warn test: arg3", "error test: arg4"} {
		if !strings.Contains(errOut.String(), msg) || strings.Contains(out.String(), msg) {
			t.Errorf("Expected %q only in stderr, got stdout=%q stderr=%q", msg, out, errOut)
		}
	}
}

// TestLoggerSource verifies that each record's source is the exact file:line of the
// logger call, not a line inside the logger package, for both stdout and stderr.
func TestLoggerSource(t *testing.T) {
	out, errOut := captureOutput(t, slog.LevelInfo)

	InfoLogger("info source test")
	_, file, infoLine, _ := runtime.Caller(0) // the line right after the InfoLogger call
	ErrorLogger("error source test")
	_, _, errorLine, _ := runtime.Caller(0) // the line right after the ErrorLogger call

	if want := fmt.Sprintf("source=%s:%d", file, infoLine-1); !strings.Contains(out.String(), want) {
		t.Errorf("Expected %q in stdout, got: %s", want, out)
	}
	if want := fmt.Sprintf("source=%s:%d", file, errorLine-1); !strings.Contains(errOut.String(), want) {
		t.Errorf("Expected %q in stderr, got: %s", want, errOut)
	}
}
