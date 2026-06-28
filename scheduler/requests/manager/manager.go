// Package manager interfaces with the system or cluster manager.
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
	managerURL  = os.Getenv("MANAGER_URL")
	managerPort = os.Getenv("MANAGER_PORT")
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

// Deploy sends the scheduling result to the system or cluster manager.
// When success is true, result is the chosen candidate's ID; when false,
// result is the negative scheduling status string.
func Deploy(jobID string, result string, success bool) error {
	url := fmt.Sprintf("%s://%s:%s%s", protocol, managerURL, managerPort, deployPath)

	var (
		payload []byte
		err     error
	)
	if success {
		payload, err = json.Marshal(deploymentRequest{JobID: jobID, CandidateID: result})
	} else {
		payload, err = json.Marshal(deploymentFailedRequest{JobID: jobID, Status: result})
	}
	if err != nil {
		logger.ErrorLogger().Println("Could not marshal deployment request")
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(payload)) //nolint:noctx
	if err != nil {
		logger.ErrorLogger().Println("Could not send deployment request")
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.ErrorLogger().Printf("Could not close response body: %v", err)
		}
	}()

	logger.DebugLogger().Printf("Deployment request %s to url %s", string(payload), url)
	return nil
}
