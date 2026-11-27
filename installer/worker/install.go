package worker

import (
	"context"
	"log/slog"

	"github.com/oakestra/oakestra/installer/worker/internal/install"
)

// Install installs an Oakestra worker on the current machine.
func Install(ctx context.Context, logger *slog.Logger, version, clusterAddr string, autoConfirm bool) error {
	return install.Run(ctx, logger, version, clusterAddr, autoConfirm)
}
