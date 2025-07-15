package main

import (
	"log"
	"scheduler/api"
	"scheduler/logger"
)

func main() {
	//Todo: Remove
	logger.SetDebugMode()

	go func() {
		StartTaskQueueServer()
		log.Fatal("Task queue server stopped")
	}()
	api.StartApiServer()
}
