package install

import (
	"context"
	"fmt"

	"github.com/oakestra/oakestra/installer/util/command"
)

func stopSystemdUnits(ctx context.Context) error {
	// The units may not exist on a fresh installation, causing an error.
	// But we do need to return errors due to CLI aborts (ctrl+c), so we check for a context error.
	for _, unit := range []string{"nodeengine", "netmanager"} {
		if err := command.RunSilent(ctx, "systemctl", "stop", unit); command.IsContextError(err) {
			return err
		}
	}
	return nil
}

func reloadSystemdUnits(ctx context.Context) error {
	if err := command.RunSilent(ctx, "systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}
	return nil
}

func startSystemdUnits(ctx context.Context) error {
	if err := command.RunSilent(ctx, "systemctl", "start", "nodeengine"); err != nil {
		return fmt.Errorf("failed to start nodeengine systemd service: %w", err)
	}
	if err := command.RunSilent(ctx, "systemctl", "start", "netmanager"); err != nil {
		return fmt.Errorf("failed to start netmanager systemd service: %w", err)
	}
	return nil
}
