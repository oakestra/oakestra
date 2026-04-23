package main

import (
	"log"
	"os"
	"scheduler/api"
	"scheduler/logger"
	"strconv"
)

func setup() {
	debugMode, err := strconv.ParseBool(os.Getenv("DEBUG"))
	if err == nil && debugMode {
		logger.SetDebugMode()
	}
}

func main() {
	setup()
	go func() {
		StartTaskQueueServer()
		log.Fatal("Task queue server stopped")
	}()
	api.StartApiServer()
}
