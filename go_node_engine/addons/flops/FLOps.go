package flops

import (
	"fmt"
	"go_node_engine/logger"
	"os/exec"
	"strings"
)

const (
	mlDataServerContainerName  = "ml_data_server"
	mlDataServerContainerImage = "ghcr.io/oakestra/addon-flops/ml-data-server:latest"
	mlDataServerPort           = "11027"
	mlDataServerVolume         = "ml_data_server_volume"
)

type FlopsAddon struct{}

// Startup initializes and starts the ML Data Server if not already running
func (a *FlopsAddon) Startup(configFiles []string) {
	log := logger.InfoLogger()
	errLog := logger.ErrorLogger()

	log.Printf("Starting FLOps ML Data Server")

	isRunning, err := a.isContainerRunning()
	if err != nil {
		errLog.Printf("Failed to check ML Data Server status: %v", err)
		return
	}

	if isRunning {
		log.Printf("FLOps ML Data Server is already running")
		return
	}

	if err := a.startMLDataServer(); err != nil {
		errLog.Printf("Failed to start ML Data Server: %v", err)
	} else {
		log.Printf("FLOps ML Data Server started successfully")
	}
}

// startMLDataServer pulls the container image and starts the ML Data Server
func (a *FlopsAddon) startMLDataServer() error {
	if err := a.pullContainerImage(); err != nil {
		return err
	}

	if err := a.runContainer(); err != nil {
		return err
	}

	return nil
}

// pullContainerImage pulls the latest ML Data Server container image
func (a *FlopsAddon) pullContainerImage() error {
	cmd := exec.Command("docker", "pull", mlDataServerContainerImage)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to pull container image: %w", err)
	}

	return nil
}

// runContainer starts the ML Data Server container with appropriate configuration
func (a *FlopsAddon) runContainer() error {
	args := []string{
		"run",
		"--rm",
		"-d",
		"-p", fmt.Sprintf("%s:%s", mlDataServerPort, mlDataServerPort),
		"-v", fmt.Sprintf("%s:/%s", mlDataServerVolume, mlDataServerVolume),
		"--name", mlDataServerContainerName,
		mlDataServerContainerImage,
	}

	cmd := exec.Command("docker", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run container: %w", err)
	}

	return nil
}

// isContainerRunning checks if the ML Data Server container is currently running
func (a *FlopsAddon) isContainerRunning() (bool, error) {
	cmd := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to list containers: %w", err)
	}

	containerNames := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, name := range containerNames {
		if strings.TrimSpace(name) == mlDataServerContainerName {
			return true, nil
		}
	}

	return false, nil
}
