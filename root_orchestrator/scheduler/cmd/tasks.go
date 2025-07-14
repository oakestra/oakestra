package main

import (
	"context"
	"encoding/json"
	"github.com/hibiken/asynq"
	"log"
	"os"
	"scheduler/calculate"
	"scheduler/calculate/schedulers/rootBestCpuMemFit"
	"scheduler/logger"
)

const TaskTypeScheduler = "schedule:job"

var RedisAddr = os.Getenv("REDIS_ADDR")

// A determines scheduler to use
type A = rootBestCpuMemFit.BestCpuMemFit

func StartTaskQueueServer() {
	redisOpt, err := asynq.ParseRedisURI(RedisAddr)
	if err != nil {
		log.Fatalf("could not parse Redis URL: %v", err)
	}

	srv := asynq.NewServer(redisOpt, asynq.Config{})

	mux := asynq.NewServeMux()
	mux.HandleFunc("scheduler:job", scheduleRequestHandler)

	if err := srv.Run(mux); err != nil {
		log.Fatal(err)
	}
}

func scheduleRequestHandler(ctx context.Context, t *asynq.Task) error {
	var algorithm A
	var jobData = algorithm.JobData()

	err := json.Unmarshal(t.Payload(), &jobData)
	if err != nil {
		logger.ErrorLogger().Println("Could not unmarshal job data")
		return err
	}
	err = calculate.PerformSchedulingRequest(jobData, algorithm)
	if err != nil {
		logger.ErrorLogger().Println("Could not Schedule job %v", err)
	}
	return err
}
