package logger

import (
	"log"
	"os"
	"sync"
)

var infologger *log.Logger
var errorlogger *log.Logger
var warnlogger *log.Logger
var warninglogger *log.Logger
var infoonce sync.Once
var erroronce sync.Once
var warnonce sync.Once
var warningonce sync.Once

// InfoLogger returns a logger for info messages
func InfoLogger() *log.Logger {
	infoonce.Do(func() {
		infologger = log.New(os.Stdout, "INFO-", log.Ldate|log.Ltime|log.Lshortfile)
	})
	return infologger
}

// WarnLogger returns a logger for warn messages
func WarnLogger() *log.Logger {
	warnonce.Do(func() {
		warnlogger = log.New(os.Stderr, "WARN-", log.Ldate|log.Ltime|log.Lshortfile)
	})
	return warnlogger
}

// ErrorLogger returns a logger for error messages
func ErrorLogger() *log.Logger {
	erroronce.Do(func() {
		errorlogger = log.New(os.Stderr, "ERROR-", log.Ldate|log.Ltime|log.Lshortfile)
	})
	return errorlogger
}

// WarningLogger returns a logger for warning messages
func WarningLogger() *log.Logger {
	warningonce.Do(func() {
		warninglogger = log.New(os.Stderr, "WARNING-", log.Ldate|log.Ltime|log.Lshortfile)
	})
	return warninglogger
}
