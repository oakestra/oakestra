package main

import (
	"log"
	"scheduler/api"
)

func main() {
	go func() {
		StartTaskQueueServer()
		log.Fatal("Task queue server stopped")
	}()
	api.StartApiServer()
}
