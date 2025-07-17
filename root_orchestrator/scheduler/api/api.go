// Package api provides endpoints for interfacing with the scheduler
package api

import (
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"log"
	"net/http"
	"os"
	"scheduler/logger"
)

const TaskTypeScheduler = "schedule:job"

var RedisAddr = os.Getenv("REDIS_ADDR")
var asynqClient *asynq.Client

func StartApiServer() {
	redisOpt, err := asynq.ParseRedisURI(RedisAddr)
	if err != nil {
		log.Fatalf("could not parse Redis URL: %v", err)
	}
	asynqClient = asynq.NewClient(redisOpt)
	defer func(asynqClient *asynq.Client) {
		err := asynqClient.Close()
		if err != nil {
			log.Fatalf("could not close asynq client: %v", err)
		}
	}(asynqClient)

	router := gin.Default()

	// register endpoints
	router.GET("/status", getStatus)
	router.POST("/api/calculate/deploy", calculate)

	// start listening
	if err := router.Run(":10004"); err != nil {
		log.Fatal(err)
	}
}

func getStatus(c *gin.Context) {
	c.Status(http.StatusOK)
}

func calculate(c *gin.Context) {
	json, err := c.GetRawData()
	if err != nil {
		logger.ErrorLogger().Printf("Received bad request: %v", err)
		c.Status(http.StatusBadRequest)
	}

	task := asynq.NewTask(TaskTypeScheduler, json)
	info, err := asynqClient.Enqueue(task)
	if err != nil {
		logger.ErrorLogger().Printf("Task enqueue error: %v", err)
		c.Status(http.StatusInternalServerError)
	}
	logger.InfoLogger().Printf("Task enqueued: ID=%s, Type=%s, Queue=%s, Payload=%s",
		info.ID, info.Type, info.Queue, string(info.Payload))
}
