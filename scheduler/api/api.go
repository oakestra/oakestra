// Package api provides endpoints for interfacing with the scheduler.
package api

import (
	"fmt"
	"net/http"
	"os"
	"scheduler/logger"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
)

// TaskTypeScheduler is the asynq task type for scheduling requests.
// It is also used by the task-queue worker in cmd/.
const TaskTypeScheduler = "schedule:job"

var (
	port        = os.Getenv("API_PORT")
	redisAddr   = os.Getenv("REDIS_ADDR")
	asynqClient *asynq.Client
)

// StartApiServer initialises the asynq client and starts the Gin HTTP server.
// It returns an error if either the Redis connection or the HTTP server fails.
func StartApiServer() error {
	redisOpt, err := asynq.ParseRedisURI(redisAddr)
	if err != nil {
		return fmt.Errorf("could not parse Redis URL: %w", err)
	}
	asynqClient = asynq.NewClient(redisOpt)
	defer func() {
		if err := asynqClient.Close(); err != nil {
			logger.ErrorLogger().Printf("could not close asynq client: %v", err)
		}
	}()

	router := gin.Default()
	router.GET("/status", getStatus)
	router.POST("/api/calculate/deploy", enqueue)

	return router.Run(fmt.Sprintf(":%v", port))
}

func getStatus(c *gin.Context) {
	c.Status(http.StatusOK)
}

func enqueue(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		logger.ErrorLogger().Printf("Received bad request: %v", err)
		c.Status(http.StatusBadRequest)
		return
	}

	task := asynq.NewTask(TaskTypeScheduler, body)
	info, err := asynqClient.Enqueue(task)
	if err != nil {
		logger.ErrorLogger().Printf("Task enqueue error: %v", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	logger.InfoLogger().Printf("Task enqueued: ID=%s, Type=%s, Queue=%s, Payload=%s",
		info.ID, info.Type, info.Queue, string(info.Payload))
}
