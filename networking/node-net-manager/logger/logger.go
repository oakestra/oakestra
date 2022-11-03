package logger

import (
	"io"
	"log"
	"os"
	"sync"
)

var infologger *log.Logger
var errorlogger *log.Logger
var debuglogger *log.Logger
var infoonce sync.Once
var erroronce sync.Once
var debugonce sync.Once
var debugMode = false

type EventType string

const (
	DEPLOYREQUEST     EventType = "DEPLOY_REQUEST"
	UNDEPLOYREQUEST   EventType = "UNDEPLOY_REQUEST"
	DEPLOYED          EventType = "DEPLOYED"
	SERVICE_RESOURCES EventType = "RESOURCES"
	NODE_RESOURCES    EventType = "RESOURCES"
	DEAD              EventType = "DEAD"
)

func SetDebugMode() {
	debugMode = true
}

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

func DebugLogger() *log.Logger {
	debugonce.Do(func() {
		debuglogger = log.New(os.Stdout, "DEBUG-", log.Ldate|log.Ltime|log.Lshortfile)
		if !debugMode {
			debuglogger.SetOutput(io.Discard)
		}
	})
	return debuglogger
}
