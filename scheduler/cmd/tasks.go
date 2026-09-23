package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"scheduler/api"
	"scheduler/calculate"
	"scheduler/calculate/schedulers/cpumemfit"
	"scheduler/logger"

	"github.com/hibiken/asynq"
)

// activeScheduler is the scheduling algorithm used at runtime.
// To switch algorithms, replace cpumemfit.Scheduler{} with another
// implementation of placement.Algorithm.
var activeScheduler = cpumemfit.Scheduler{}

var redisAddr = os.Getenv("REDIS_ADDR")

// StartTaskQueueServer connects to Redis and begins processing scheduling tasks.
// It blocks until the server stops, returning any error.
func StartTaskQueueServer() error {
	redisOpt, err := asynq.ParseRedisURI(redisAddr)
	if err != nil {
		return fmt.Errorf("could not parse Redis URL: %w", err)
	}

	srv := asynq.NewServer(redisOpt, asynq.Config{})

	mux := asynq.NewServeMux()
	mux.HandleFunc(api.TaskTypeScheduler, scheduleRequestHandler)

	return srv.Run(mux)
}

func scheduleRequestHandler(ctx context.Context, t *asynq.Task) error {
	jobData := activeScheduler.JobData()

	logger.DebugLogger().Printf("Received payload: %v", string(t.Payload()))
	if err := json.Unmarshal(t.Payload(), &jobData); err != nil {
		logger.ErrorLogger().Printf("Could not unmarshal job data: %v", err)
		return err
	}
	logger.DebugLogger().Printf("Received job data: %v", jobData)

	if err := calculate.PerformSchedulingRequest(jobData, activeScheduler); err != nil {
		logger.ErrorLogger().Printf("Could not schedule job: %v", err)
		return err
	}
	return nil
}
