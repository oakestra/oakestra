package cluster

import (
	"errors"
	"log/slog"
)

// Install is the entry point for cluster orchestrator installation.
func Install(logger *slog.Logger) error {
	const message = "cluster installation is not yet implemented"
	logger.Error(message)
	return errors.New(message)
}
