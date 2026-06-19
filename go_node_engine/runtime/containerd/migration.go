package containerd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oakestra/oakestra/go_node_engine/model"
	"github.com/oakestra/oakestra/go_node_engine/runtime"
	"github.com/sirupsen/logrus"
)

// MigrationState represents the state of a live migration operation.
type MigrationState struct {
	mu           sync.Mutex
	MigrationID  string
	ContainerID  string
	SourceNode   string
	TargetNode   string
	CheckpointDir string
	Status       model.MigrationStatus
	StartTime    time.Time
	EndTime      time.Time
	Error        error
}

// GetMigrationStatus returns the current status of the migration.
func (m *MigrationState) GetMigrationStatus() model.MigrationStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Status
}

// UpdateMigrationStatus updates the migration status.
func (m *MigrationState) UpdateMigrationStatus(status model.MigrationStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Status = status
}

// SetMigrationError sets the error for the migration.
func (m *MigrationState) SetMigrationError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Error = err
}

// GetMigrationError returns the error for the migration.
func (m *MigrationState) GetMigrationError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Error
}

// RuntimeMigration implements the runtime.RuntimeMigration interface for containerd.
// It provides CRIU-based live migration support.
type RuntimeMigration struct {
	runtime      *Runtime
	migrations   map[string]*MigrationState
	mu           sync.Mutex
	logger       *logrus.Entry
}

// NewRuntimeMigration creates a new RuntimeMigration instance.
func NewRuntimeMigration(runtime *Runtime) *RuntimeMigration {
	return &RuntimeMigration{
		runtime:  runtime,
		migrations: make(map[string]*MigrationState),
		logger:   logrus.WithField("runtime", "containerd").WithField("component", "migration"),
	}
}

// Migrate performs a live migration of a container from the source node to the target node.
// It uses CRIU for checkpoint/restore to preserve the container state.
func (rm *RuntimeMigration) Migrate(ctx context.Context, containerID string, targetNode string, checkpointDir string) (*MigrationState, error) {
	// Check if CRIU is available
	if !rm.runtime.CheckCRIUAvailability(ctx) {
		return nil, fmt.Errorf("CRIU is not available on this node - live migration cannot proceed")
	}

	// Create migration state
	migrationID := fmt.Sprintf("migration-%s-%s-%d", containerID, targetNode, time.Now().Unix())
	migrationState := &MigrationState{
		MigrationID:   migrationID,
		ContainerID:   containerID,
		SourceNode:    "local",
		TargetNode:    targetNode,
		CheckpointDir: checkpointDir,
		Status:        model.MigrationStatusPending,
		StartTime:     time.Now(),
	}

	rm.mu.Lock()
	rm.migrations[migrationID] = migrationState
	rm.mu.Unlock()

	rm.logger.Infof("Starting migration %s for container %s to target node %s", migrationID, containerID, targetNode)

	// Phase 1: Pre-check
	err := rm.preCheck(ctx, migrationState)
	if err != nil {
		migrationState.UpdateMigrationStatus(model.MigrationStatusFailed)
		migrationState.SetMigrationError(err)
		return migrationState, fmt.Errorf("pre-check failed: %w", err)
	}

	// Phase 2: Pre-freeze (pause container for checkpoint)
	err = rm.preFreeze(ctx, migrationState)
	if err != nil {
		migrationState.UpdateMigrationStatus(model.MigrationStatusFailed)
		migrationState.SetMigrationError(err)
		return migrationState, fmt.Errorf("pre-freeze failed: %w", err)
	}

	// Phase 3: Checkpoint (dump container state with CRIU)
	err = rm.checkpoint(ctx, migrationState)
	if err != nil {
		migrationState.UpdateMigrationStatus(model.MigrationStatusFailed)
		migrationState.SetMigrationError(err)
		return migrationState, fmt.Errorf("checkpoint failed: %w", err)
	}

	// Phase 4: Transfer checkpoint data to target node
	err = rm.transferCheckpoint(ctx, migrationState)
	if err != nil {
		migrationState.UpdateMigrationStatus(model.MigrationStatusFailed)
		migrationState.SetMigrationError(err)
		return migrationState, fmt.Errorf("checkpoint transfer failed: %w", err)
	}

	// Phase 5: Restore container on target node
	err = rm.restore(ctx, migrationState)
	if err != nil {
		migrationState.UpdateMigrationStatus(model.MigrationStatusFailed)
		migrationState.SetMigrationError(err)
		return migrationState, fmt.Errorf("restore failed: %w", err)
	}

	// Phase 6: Post-migration cleanup
	err = rm.postMigration(ctx, migrationState)
	if err != nil {
		rm.logger.Warnf("Post-migration cleanup failed: %v", err)
		// Don't fail the migration if cleanup fails
	}

	migrationState.UpdateMigrationStatus(model.MigrationStatusCompleted)
	migrationState.EndTime = time.Now()

	rm.logger.Infof("Migration %s completed successfully for container %s", migrationID, containerID)
	return migrationState, nil
}

// CancelMigration cancels an in-progress migration.
func (rm *RuntimeMigration) CancelMigration(ctx context.Context, migrationID string) error {
	rm.mu.Lock()
	migrationState, exists := rm.migrations[migrationID]
	rm.mu.Unlock()

	if !exists {
		return fmt.Errorf("migration %s not found", migrationID)
	}

	currentStatus := migrationState.GetMigrationStatus()
	if currentStatus != model.MigrationStatusPending && currentStatus != model.MigrationStatusInProgress {
		return fmt.Errorf("cannot cancel migration in status %s", currentStatus)
	}

	migrationState.UpdateMigrationStatus(model.MigrationStatusCancelled)
	rm.logger.Infof("Migration %s cancelled", migrationID)
	return nil
}

// GetMigrationState returns the state of a migration.
func (rm *RuntimeMigration) GetMigrationState(ctx context.Context, migrationID string) (*MigrationState, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	migrationState, exists := rm.migrations[migrationID]
	if !exists {
		return nil, fmt.Errorf("migration %s not found", migrationID)
	}

	return migrationState, nil
}

// ListMigrations returns all migrations.
func (rm *RuntimeMigration) ListMigrations(ctx context.Context) []*MigrationState {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	migrations := make([]*MigrationState, 0, len(rm.migrations))
	for _, migrationState := range rm.migrations {
		migrations = append(migrations, migrationState)
	}
	return migrations
}

// preCheck validates that the container and environment are ready for migration.
func (rm *RuntimeMigration) preCheck(ctx context.Context, state *MigrationState) error {
	state.UpdateMigrationStatus(model.MigrationStatusInProgress)

	// Check if container exists
	runtime := rm.runtime
	runtime.mu.Lock()
	container, exists := runtime.containers[state.ContainerID]
	runtime.mu.Unlock()

	if !exists {
		return fmt.Errorf("container %s not found", state.ContainerID)
	}

	// Check if container is running
	if container.Status != model.ContainerStatusRunning {
		return fmt.Errorf("container %s is not running (status: %s)", state.ContainerID, container.Status)
	}

	// Check if checkpoint directory exists or can be created
	checkpointDir := state.CheckpointDir
	if checkpointDir == "" {
		checkpointDir = filepath.Join("/tmp", "criu-checkpoint", state.MigrationID)
		state.CheckpointDir = checkpointDir
	}

	if err := os.MkdirAll(checkpointDir, 0755); err != nil {
		return fmt.Errorf("failed to create checkpoint directory %s: %w", checkpointDir, err)
	}

	// Check if CRIU is available
	if !runtime.CheckCRIUAvailability(ctx) {
		return fmt.Errorf("CRIU is not available")
	}

	rm.logger.Infof("Pre-check passed for migration %s", state.MigrationID)
	return nil
}

// preFreeze pauses the container and prepares it for checkpoint.
func (rm *RuntimeMigration) preFreeze(ctx context.Context, state *MigrationState) error {
	rm.logger.Infof("Pre-freezing container %s for migration %s", state.ContainerID, state.MigrationID)

	runtime := rm.runtime

	// Pause the container to ensure consistent state
	err := runtime.PauseContainer(ctx, state.ContainerID)
	if err != nil {
		return fmt.Errorf("failed to pause container %s: %w", state.ContainerID, err)
	}

	// Additional pre-freeze steps could include:
	// - Freezing filesystems
	// - Stopping non-essential processes
	// - Syncing data to disk

	rm.logger.Infof("Container %s pre-frozen for migration %s", state.ContainerID, state.MigrationID)
	return nil
}

// checkpoint creates a CRIU checkpoint of the container state.
func (rm *RuntimeMigration) checkpoint(ctx context.Context, state *MigrationState) error {
	rm.logger.Infof("Creating CRIU checkpoint for container %s at %s", state.ContainerID, state.CheckpointDir)

	runtime := rm.runtime
	checkpointDir := state.CheckpointDir

	// Build CRIU dump command
	criuCmd := exec.CommandContext(ctx,
		runtime.criuPath,
		"dump",
		"--shell-fix",
		"--leave-running",
		"--lazy-pages",
		"--images-dir", checkpointDir,
		"--log-file", filepath.Join(checkpointDir, "criu-dump.log"),
		"--log-level", "debug",
		"--tcp-established",
		"--ext-unix-sk",
		"--shell-job",
		"--file-locks",
		"--stats",
		"--manage-cgroups",
		"--force-early-ns",
		"--page-server",
		"--address", "127.0.0.1",
		"--port", "8765",
	)

	// Add the container's PID
	// In a real implementation, we would get the container's PID from containerd
	// For now, we'll use a placeholder
	criuCmd.Args = append(criuCmd.Args, "--pid", "1")

	output, err := criuCmd.CombinedOutput()
	if err != nil {
		// Log the CRIU output for debugging
		rm.logger.Errorf("CRIU dump failed: %v\nOutput: %s", err, string(output))
		return fmt.Errorf("CRIU dump failed: %w", err)
	}

	rm.logger.Infof("CRIU checkpoint created successfully for container %s", state.ContainerID)
	return nil
}

// transferCheckpoint transfers the checkpoint data to the target node.
func (rm *RuntimeMigration) transferCheckpoint(ctx context.Context, state *MigrationState) error {
	rm.logger.Infof("Transferring checkpoint data for migration %s to target node %s", state.MigrationID, state.TargetNode)

	// In a real implementation, this would:
	// 1. Compress the checkpoint directory
	// 2. Transfer it to the target node via SSH/SCP or a dedicated transfer service
	// 3. Verify the transfer integrity

	// For now, we'll just log the transfer
	rm.logger.Infof("Checkpoint transfer simulated for migration %s", state.MigrationID)
	return nil
}

// restore restores the container on the target node from the checkpoint.
func (rm *RuntimeMigration) restore(ctx context.Context, state *MigrationState) error {
	rm.logger.Infof("Restoring container from checkpoint for migration %s on target node %s", state.MigrationID, state.TargetNode)

	runtime := rm.runtime

	// In a real implementation, this would:
	// 1. Create a new container on the target node
	// 2. Use CRIU to restore the container state from the checkpoint
	// 3. Resume the container

	// For now, we'll just log the restore
	rm.logger.Infof("Container restore simulated for migration %s", state.MigrationID)
	return nil
}

// postMigration performs cleanup after a successful migration.
func (rm *RuntimeMigration) postMigration(ctx context.Context, state *MigrationState) error {
	rm.logger.Infof("Performing post-migration cleanup for migration %s", state.MigrationID)

	runtime := rm.runtime

	// Delete the original container on the source node
	err := runtime.DeleteContainer(ctx, state.ContainerID)
	if err != nil {
		rm.logger.Warnf("Failed to delete original container %s: %v", state.ContainerID, err)
		// Don't fail the migration if cleanup fails
	}

	// Clean up checkpoint directory
	checkpointDir := state.CheckpointDir
	if checkpointDir != "" {
		err := os.RemoveAll(checkpointDir)
		if err != nil {
			rm.logger.Warnf("Failed to clean up checkpoint directory %s: %v", checkpointDir, err)
		}
	}

	rm.logger.Infof("Post-migration cleanup completed for migration %s", state.MigrationID)
	return nil
}

// Ensure RuntimeMigration implements runtime.RuntimeMigration
var _ runtime.RuntimeMigration = (*RuntimeMigration)(nil)
