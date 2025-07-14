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
	SYSTEM_MANAGER_URL  = os.Getenv("SYSTEM_MANAGER_URL")
	SYSTEM_MANAGER_PORT = os.Getenv("SYSTEM_MANAGER_PORT")
)

const (
	PROTOCOL = "http"
	DEPLOY   = "/api/result/deploy"
)

type deploymentRequest struct {
	JobId     string `json:"job_id"`
	ClusterId string `json:"cluster_id"`
}

// Deploy sends the scheduling result to the system or cluster manager
func Deploy(jobID string, clusterID string) error {
	url := fmt.Sprintf("%s://%s:%s%s", PROTOCOL, SYSTEM_MANAGER_URL, SYSTEM_MANAGER_PORT, DEPLOY)

	req := deploymentRequest{jobID, clusterID}
	payload, err := json.Marshal(req)
	if err != nil {
		logger.ErrorLogger().Println("Could not marshal deployment request")
		return err
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
