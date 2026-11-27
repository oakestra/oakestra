package install

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/oakestra/oakestra/installer/util/command"
	"github.com/oakestra/oakestra/installer/util/elevate"
	"github.com/oakestra/oakestra/installer/util/prompt"
)

type installer struct {
	logger      *slog.Logger
	autoConfirm bool
}

// Run installs the worker components and optionally starts their services.
func Run(ctx context.Context, logger *slog.Logger, version, clusterAddr string, autoConfirm bool) error {
	var address clusterAddress
	if strings.TrimSpace(clusterAddr) != "" {
		var err error
		address, err = parseClusterAddress(clusterAddr)
		if err != nil {
			return err
		}
	} else if autoConfirm {
		return errors.New("--cluster-addr is required with --yes")
	}

	if err := checkPrerequisites(); err != nil {
		return err
	}
	if !autoConfirm && !prompt.PromptAvailable() {
		return errors.New("worker installation needs a terminal for confirmations; pass --yes to install non-interactively")
	}

	if err := elevate.RequireRoot(ctx, logger, autoConfirm); err != nil {
		return err
	}
	worker := installer{logger: logger, autoConfirm: autoConfirm}
	var selectedCluster clusterProbe
	if address.host != "" {
		var err error
		selectedCluster, err = probeDirectCluster(ctx, address)
		if err != nil {
			return err
		}
	}

	osFamily := detectOSFamily()
	if osFamily == Unknown {
		logger.WarnContext(ctx, "Distribution is unsupported for automatic NVIDIA Container Toolkit installation")
	}

	containerdReady, err := worker.installContainerd(ctx)
	if err != nil {
		return err
	}
	if !containerdReady {
		logger.WarnContext(ctx, "Containerd is unavailable. Install and enable it before using the containerd runtime")
	} else if err := worker.installNvidiaCTK(ctx, osFamily); err != nil {
		return err
	}
	workerReady, err := worker.installFirstParty(ctx, version)
	if err != nil {
		return err
	}
	if !workerReady {
		logger.WarnContext(ctx, "Oakestra worker was not installed")
		return nil
	}

	if address.host == "" {
		selectedCluster, err = worker.selectCluster(ctx)
	}
	if err == nil {
		err = worker.configureCluster(ctx, selectedCluster)
	}
	if err != nil {
		if autoConfirm || errors.Is(err, context.Canceled) {
			return err
		}
		logger.WarnContext(ctx, "Failed to configure worker cluster", "err", err)
	}
	if err := worker.startWorker(ctx); err != nil {
		if command.IsContextError(err) {
			return err
		}
		logger.WarnContext(ctx, "Failed to start worker", "err", err)
	}
	logger.InfoContext(ctx, "Oakestra worker installed")
	return nil
}

func (i installer) startWorker(ctx context.Context) error {
	if !i.autoConfirm {
		confirmed, err := prompt.Confirm(ctx, "Start the worker node now?")
		if err != nil || !confirmed {
			return err
		}
	}
	return startSystemdUnits(ctx)
}
