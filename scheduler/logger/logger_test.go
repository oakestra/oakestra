package logger

import (
	"bytes"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"testing"
)

// captureOutput redirects the stdout and stderr loggers into buffers for the duration of the
// test. The buffers use the package level, so Init(true) still applies.
func captureOutput(t *testing.T) (out, errOut *bytes.Buffer) {
	t.Helper()
	origStdout, origStderr := stdout, stderr
	t.Cleanup(func() {
		stdout, stderr = origStdout, origStderr
		level.Set(slog.LevelInfo)
	})

	out, errOut = &bytes.Buffer{}, &bytes.Buffer{}
	stdout = slog.New(slog.NewTextHandler(out, opts))
	stderr = slog.New(slog.NewTextHandler(errOut, opts))
	return out, errOut
}

// TestInitDebug verifies that DebugLogger only outputs after Init(true) is called
func TestInitDebug(t *testing.T) {
	out, _ := captureOutput(t)

	DebugLogger("before Init(true)")
	if strings.Contains(out.String(), "before Init(true)") {
		t.Error("DebugLogger should not output before Init(true)")
	}

	Init(true)
	DebugLogger("after Init(true)")
	if !strings.Contains(out.String(), "after Init(true)") {
		t.Errorf("DebugLogger should output after Init(true), got: %s", out)
	}
}

// TestLoggerFunctions verifies that INFO/DEBUG go to stdout and WARN/ERROR go to stderr.
func TestLoggerFunctions(t *testing.T) {
	out, errOut := captureOutput(t)
	Init(true)

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
	out, errOut := captureOutput(t)

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
