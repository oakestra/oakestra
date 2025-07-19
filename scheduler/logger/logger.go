package logger

import (
	"io"
	"log"
	"os"
	"sync"
)

var infoLogger *log.Logger
var errorLogger *log.Logger
var debugLogger *log.Logger
var infoOnce sync.Once
var errorOnce sync.Once
var debugOnce sync.Once
var debugMode = false

func SetDebugMode() {
	debugMode = true
}

func InfoLogger() *log.Logger {
	infoOnce.Do(func() {
		infoLogger = log.New(os.Stdout, "INFO-", log.Ldate|log.Ltime|log.Lshortfile)
	})
	return infoLogger
}

func ErrorLogger() *log.Logger {
	errorOnce.Do(func() {
		errorLogger = log.New(os.Stderr, "ERROR-", log.Ldate|log.Ltime|log.Lshortfile)
	})
	return errorLogger
}

func DebugLogger() *log.Logger {
	debugOnce.Do(func() {
		debugLogger = log.New(os.Stdout, "DEBUG-", log.Ldate|log.Ltime|log.Lshortfile)
		if !debugMode {
			debugLogger.SetOutput(io.Discard)
		}
	})
	return debugLogger
}
