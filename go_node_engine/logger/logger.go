package logger

import (
	"log"
	"os"
	"sync"
)

var infologger *log.Logger
var errorlogger *log.Logger
var infoonce sync.Once
var erroronce sync.Once

func InfoLogger() *log.Logger {
	infoonce.Do(func() {
		infologger = log.New(os.Stdout, "INFO-", log.Ldate|log.Ltime|log.Lshortfile)
	})
	return infologger
}

func ErrorLogger() *log.Logger {
	erroronce.Do(func() {
		errorlogger = log.New(os.Stderr, "ERROR-", log.Ldate|log.Ltime|log.Lshortfile)
	})
	return errorlogger
}
