// Package manager interfaces with the system or cluster manager
package manager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"scheduler/logger"
)

var (
	MANAGER_URL  = os.Getenv("MANAGER_URL")
	MANAGER_PORT = os.Getenv("MANAGER_PORT")
)

const (
	PROTOCOL = "http"
	DEPLOY   = "/api/result/deploy"
)

type deploymentRequest struct {
	JobId     string `json:"job_id"`
	ClusterId string `json:"cluster_id"`
}

type deploymentFailedRequest struct {
	JobId  string `json:"job_id"`
	Status string `json:"status"`
}

// Deploy sends the scheduling result to the system or cluster manager
func Deploy(jobId string, result string, success bool) error {
	url := fmt.Sprintf("%s://%s:%s%s", PROTOCOL, MANAGER_URL, MANAGER_PORT, DEPLOY)

	var payload []byte
	var err error
	if success {
		req := deploymentRequest{jobId, result}
		payload, err = json.Marshal(req)
		if err != nil {
			logger.ErrorLogger().Println("Could not marshal deployment request")
			return err
		}
	} else {
		req := deploymentFailedRequest{jobId, result}
		payload, err = json.Marshal(req)
		if err != nil {
			logger.ErrorLogger().Println("Could not marshal deployment request")
			return err
		}
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		logger.ErrorLogger().Println("Could not send deployment request")
		return err
	}

	err = resp.Body.Close()
	if err != nil {
		return err
	}
	return nil
}
