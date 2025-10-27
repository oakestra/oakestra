package main

import (
	"log"
	"scheduler/api"
	"scheduler/logger"
)

func main() {
	go func() {
		StartTaskQueueServer()
		log.Fatal("Task queue server stopped")
	}()
	logger.SetDebugMode()
	api.StartApiServer()
}
