package main

import (
	"log"
	"os"
	"scheduler/api"
	"scheduler/logger"
	"strconv"
)

func setup() {
	debug, err := strconv.ParseBool(os.Getenv("DEBUG"))
	if err != nil {
		debug = false
	}
	logger.Init(debug)
}

func main() {
	setup()

	errc := make(chan error, 2)
	go func() { errc <- StartTaskQueueServer() }()
	go func() { errc <- api.StartApiServer() }()

	// Block until one of the servers stops (expected to run forever).
	// log.Fatal is intentional here: it is the single top-level exit point.
	log.Fatal(<-errc)
}
