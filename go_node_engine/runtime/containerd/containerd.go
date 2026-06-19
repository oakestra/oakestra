package containerd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/oakestra/oakestra/go_node_engine/model"
	"github.com/oakestra/oakestra/go_node_engine/runtime"
	"github.com/sirupsen/logrus"
)

const (
	RuntimeName = "containerd"
)

// Runtime implements the runtime.Runtime interface using containerd.
// It also optionally implements runtime.RuntimeMigration for live migration support.
type Runtime struct {
	mu          sync.Mutex
	containers  map[string]*Container
	containerd  string
	namespace   string
	rootDir     string
	criuPath    string
	logger      *logrus.Entry
}

// NewRuntime creates a new containerd runtime instance.
func NewRuntime(ctx context.Context, rootDir string, namespace string) (runtime.Runtime, error) {
	// Check containerd binary
	containerdPath, err := exec.LookPath("containerd")
	if err != nil {
		return nil, fmt.Errorf("containerd binary not found: %w", err)
	}

	// Check CRIU binary for migration support
	criuPath, err := exec.LookPath("criu")
	if err != nil {
		// CRIU not available - migration will not be supported
		logrus.Warn("CRIU binary not found - live migration will not be available")
	}

	r := &Runtime{
		containers:  make(map[string]*Container),
		containerd:  containerdPath,
		namespace:   namespace,
		rootDir:     rootDir,
		criuPath:    criuPath,
		logger:      logrus.WithField("runtime", RuntimeName),
	}

	// Initialize containerd client and verify connection
	err = r.initContainerd(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize containerd: %w", err)
	}

	r.logger.Info("Containerd runtime initialized")
	return r, nil
}

// initContainerd verifies the containerd connection and creates necessary directories.
func (r *Runtime) initContainerd(ctx context.Context) error {
	// Create root directory if it doesn't exist
	if err := os.MkdirAll(r.rootDir, 0755); err != nil {
		return fmt.Errorf("failed to create root directory %s: %w", r.rootDir, err)
	}

	// Verify containerd is running by checking its socket
	cmd := exec.CommandContext(ctx, r.containerd, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run containerd --version: %w, output: %s", err, string(output))
	}

	r.logger.Debugf("Containerd version: %s", strings.TrimSpace(string(output)))
	return nil
}

// CreateContainer creates a new container with the given configuration.
func (r *Runtime) CreateContainer(ctx context.Context, config *runtime.ContainerConfig) (*Container, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if container already exists
	if _, exists := r.containers[config.ContainerID]; exists {
		return nil, fmt.Errorf("container %s already exists", config.ContainerID)
	}

	// Create container using containerd CLI
	cmd := exec.CommandContext(ctx,
		r.containerd,
		"run",
		"--namespace", r.namespace,
		"--id", config.ContainerID,
		"--root", filepath.Join(r.rootDir, "containers", config.ContainerID),
		config.Image,
	)

	if len(config.Command) > 0 {
		cmd.Args = append(cmd.Args, config.Command...)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to create container %s: %w, output: %s", config.ContainerID, err, string(output))
	}

	container := &Container{
		ID:         config.ContainerID,
		Image:      config.Image,
		Status:     model.ContainerStatusCreated,
		CreateTime: time.Now(),
		Runtime:    r,
	}

	r.containers[config.ContainerID] = container
	r.logger.Infof("Container %s created from image %s", config.ContainerID, config.Image)
	return container, nil
}

// StartContainer starts a created container.
func (r *Runtime) StartContainer(ctx context.Context, containerID string) error {
	r.mu.Lock()
	container, exists := r.containers[containerID]
	r.mu.Unlock()

	if !exists {
		return fmt.Errorf("container %s not found", containerID)
	}

	// Start container using containerd CLI
	cmd := exec.CommandContext(ctx,
		r.containerd,
		"start",
		"--namespace", r.namespace,
		"--id", containerID,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start container %s: %w, output: %s", containerID, err, string(output))
	}

	r.mu.Lock()
	container.Status = model.ContainerStatusRunning
	r.mu.Unlock()

	r.logger.Infof("Container %s started", containerID)
	return nil
}

// StopContainer stops a running container.
func (r *Runtime) StopContainer(ctx context.Context, containerID string, timeout int) error {
	r.mu.Lock()
	container, exists := r.containers[containerID]
	r.mu.Unlock()

	if !exists {
		return fmt.Errorf("container %s not found", containerID)
	}

	// Stop container using containerd CLI
	timeoutStr := fmt.Sprintf("%ds", timeout)
	cmd := exec.CommandContext(ctx,
		r.containerd,
		"stop",
		"--namespace", r.namespace,
		"--id", containerID,
		"--timeout", timeoutStr,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// If container is already stopped, that's okay
		if !strings.Contains(string(output), "already stopped") && !strings.Contains(string(output), "no such process") {
			return fmt.Errorf("failed to stop container %s: %w, output: %s", containerID, err, string(output))
		}
	}

	r.mu.Lock()
	container.Status = model.ContainerStatusStopped
	r.mu.Unlock()

	r.logger.Infof("Container %s stopped", containerID)
	return nil
}

// DeleteContainer deletes a container.
func (r *Runtime) DeleteContainer(ctx context.Context, containerID string) error {
	r.mu.Lock()
	container, exists := r.containers[containerID]
	r.mu.Unlock()

	if !exists {
		return fmt.Errorf("container %s not found", containerID)
	}

	// Delete container using containerd CLI
	cmd := exec.CommandContext(ctx,
		r.containerd,
		"rm",
		"--namespace", r.namespace,
		"--id", containerID,
		"--force",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete container %s: %w, output: %s", containerID, err, string(output))
	}

	r.mu.Lock()
	delete(r.containers, containerID)
	r.mu.Unlock()

	r.logger.Infof("Container %s deleted", containerID)
	return nil
}

// GetContainerStatus returns the status of a container.
func (r *Runtime) GetContainerStatus(ctx context.Context, containerID string) (*model.ContainerStatus, error) {
	r.mu.Lock()
	container, exists := r.containers[containerID]
	r.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("container %s not found", containerID)
	}

	// Query containerd for actual status
	cmd := exec.CommandContext(ctx,
		r.containerd,
		"info",
		"--namespace", r.namespace,
		"--id", containerID,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Container might not exist in containerd
		r.logger.Warnf("Failed to get status for container %s: %v", containerID, err)
		return &model.ContainerStatus{
			ID:     containerID,
			Status: model.ContainerStatusNotFound,
		}, nil
	}

	status := container.Status
	if strings.Contains(string(output), "running") {
		status = model.ContainerStatusRunning
	} else if strings.Contains(string(output), "stopped") {
		status = model.ContainerStatusStopped
	}

	return &model.ContainerStatus{
		ID:     containerID,
		Status: status,
	}, nil
}

// ListContainers returns all containers managed by this runtime.
func (r *Runtime) ListContainers(ctx context.Context) ([]*Container, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	containers := make([]*Container, 0, len(r.containers))
	for _, container := range r.containers {
		containers = append(containers, container)
	}
	return containers, nil
}

// Name returns the name of this runtime.
func (r *Runtime) Name() string {
	return RuntimeName
}

// GetCriuPath returns the path to the CRIU binary.
// Returns empty string if CRIU is not available.
func (r *Runtime) GetCriuPath() string {
	return r.criuPath
}

// CheckCRIUAvailability checks if CRIU is available and functional.
func (r *Runtime) CheckCRIUAvailability(ctx context.Context) bool {
	if r.criuPath == "" {
		return false
	}

	cmd := exec.CommandContext(ctx, r.criuPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		r.logger.Warnf("CRIU version check failed: %v, output: %s", err, string(output))
		return false
	}

	r.logger.Debugf("CRIU version: %s", strings.TrimSpace(string(output)))
	return true
}

// GetContainerdPath returns the path to the containerd binary.
func (r *Runtime) GetContainerdPath() string {
	return r.containerd
}

// GetNamespace returns the containerd namespace used by this runtime.
func (r *Runtime) GetNamespace() string {
	return r.namespace
}

// GetRootDir returns the root directory for containerd data.
func (r *Runtime) GetRootDir() string {
	return r.rootDir
}

// GetContainers returns the internal containers map (for testing).
func (r *Runtime) GetContainers() map[string]*Container {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.containers
}

// KillContainer sends a signal to a container.
func (r *Runtime) KillContainer(ctx context.Context, containerID string, signal syscall.Signal) error {
	r.mu.Lock()
	container, exists := r.containers[containerID]
	r.mu.Unlock()

	if !exists {
		return fmt.Errorf("container %s not found", containerID)
	}

	// Send signal to container process
	cmd := exec.CommandContext(ctx,
		r.containerd,
		"kill",
		"--namespace", r.namespace,
		"--id", containerID,
		fmt.Sprintf("%d", signal),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to kill container %s with signal %d: %w, output: %s", containerID, signal, err, string(output))
	}

	r.logger.Infof("Container %s killed with signal %d", containerID, signal)
	return nil
}

// ExecInContainer executes a command inside a running container.
func (r *Runtime) ExecInContainer(ctx context.Context, containerID string, cmd []string, env map[string]string) ([]byte, error) {
	r.mu.Lock()
	container, exists := r.containers[containerID]
	r.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("container %s not found", containerID)
	}

	if container.Status != model.ContainerStatusRunning {
		return nil, fmt.Errorf("container %s is not running (status: %s)", containerID, container.Status)
	}

	// Build exec command
	execCmd := exec.CommandContext(ctx,
		r.containerd,
		"exec",
		"--namespace", r.namespace,
		"--id", containerID,
	)

	// Add environment variables
	for k, v := range env {
		execCmd.Env = append(execCmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	execCmd.Args = append(execCmd.Args, cmd...)

	output, err := execCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to exec in container %s: %w, output: %s", containerID, err, string(output))
	}

	return output, nil
}

// PauseContainer pauses a running container.
func (r *Runtime) PauseContainer(ctx context.Context, containerID string) error {
	r.mu.Lock()
	container, exists := r.containers[containerID]
	r.mu.Unlock()

	if !exists {
		return fmt.Errorf("container %s not found", containerID)
	}

	// Pause container using containerd CLI
	cmd := exec.CommandContext(ctx,
		r.containerd,
		"pause",
		"--namespace", r.namespace,
		"--id", containerID,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to pause container %s: %w, output: %s", containerID, err, string(output))
	}

	r.mu.Lock()
	container.Status = model.ContainerStatusPaused
	r.mu.Unlock()

	r.logger.Infof("Container %s paused", containerID)
	return nil
}

// ResumeContainer resumes a paused container.
func (r *Runtime) ResumeContainer(ctx context.Context, containerID string) error {
	r.mu.Lock()
	container, exists := r.containers[containerID]
	r.mu.Unlock()

	if !exists {
		return fmt.Errorf("container %s not found", containerID)
	}

	// Resume container using containerd CLI
	cmd := exec.CommandContext(ctx,
		r.containerd,
		"resume",
		"--namespace", r.namespace,
		"--id", containerID,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to resume container %s: %w, output: %s", containerID, err, string(output))
	}

	r.mu.Lock()
	container.Status = model.ContainerStatusRunning
	r.mu.Unlock()

	r.logger.Infof("Container %s resumed", containerID)
	return nil
}
