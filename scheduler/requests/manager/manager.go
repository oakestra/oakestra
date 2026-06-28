// Package manager interfaces with the system or cluster manager.
package manager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"scheduler/logger"
	"time"
)

var (
	managerURL  = os.Getenv("MANAGER_URL")
	managerPort = os.Getenv("MANAGER_PORT")
	// deployURL is built once at startup; host and port are fixed after process start.
	deployURL     = fmt.Sprintf("%s://%s:%s%s", protocol, managerURL, managerPort, deployPath)
	managerClient = &http.Client{Timeout: 10 * time.Second}
)

const (
	protocol   = "http"
	deployPath = "/api/result/deploy"
)

type deploymentRequest struct {
	JobID       string `json:"job_id"`
	CandidateID string `json:"candidate_id"`
}

type deploymentFailedRequest struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
}

// DeploySuccess notifies the manager that job jobID was placed on candidateID.
func DeploySuccess(jobID, candidateID string) error {
	return doPost(deploymentRequest{JobID: jobID, CandidateID: candidateID})
}

// DeployFailed notifies the manager that scheduling job jobID failed with
// the given negative status string.
func DeployFailed(jobID, status string) error {
	return doPost(deploymentFailedRequest{JobID: jobID, Status: status})
}

func doPost(payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		logger.ErrorLogger().Println("Could not marshal deployment request")
		return err
	}

	resp, err := managerClient.Post(deployURL, "application/json", bytes.NewBuffer(data))
	if err != nil {
		logger.ErrorLogger().Println("Could not send deployment request")
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.ErrorLogger().Printf("Could not close response body: %v", err)
		}
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("manager returned HTTP %d", resp.StatusCode)
	}

	logger.DebugLogger().Printf("Deployment request %s to url %s", string(data), deployURL)
	return nil
}
