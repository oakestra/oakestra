package root

import (
	"errors"
	"log/slog"
)

// Install is the entry point for root orchestrator installation.
func Install(logger *slog.Logger) error {
	const message = "root installation is not yet implemented"
	logger.Error(message)
	return errors.New(message)
}
