// Package logger provides the shared loggers used throughout the scheduler.
package logger

import (
	"io"
	"log"
	"os"
)

// Loggers are initialised to safe defaults at package load time so that
// library code can log without requiring an explicit Init call. Debug output
// is discarded unless Init is called with debug=true.
var (
	infoLogger  = log.New(os.Stdout, "INFO-", log.Ldate|log.Ltime|log.Lshortfile)
	errorLogger = log.New(os.Stderr, "ERROR-", log.Ldate|log.Ltime|log.Lshortfile)
	debugLogger = log.New(io.Discard, "DEBUG-", log.Ldate|log.Ltime|log.Lshortfile)
)

// Init reconfigures the loggers. Call once from main. If debug is true,
// debug output is written to stdout instead of being discarded.
func Init(debug bool) {
	infoLogger = log.New(os.Stdout, "INFO-", log.Ldate|log.Ltime|log.Lshortfile)
	errorLogger = log.New(os.Stderr, "ERROR-", log.Ldate|log.Ltime|log.Lshortfile)

	debugOut := io.Discard
	if debug {
		debugOut = os.Stdout
	}
	debugLogger = log.New(debugOut, "DEBUG-", log.Ldate|log.Ltime|log.Lshortfile)
}

// InfoLogger returns the info logger.
func InfoLogger() *log.Logger { return infoLogger }

// ErrorLogger returns the error logger.
func ErrorLogger() *log.Logger { return errorLogger }

// DebugLogger returns the debug logger.
func DebugLogger() *log.Logger { return debugLogger }
