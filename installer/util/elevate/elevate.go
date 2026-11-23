package elevate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/oakestra/oakestra/installer/util/enact"
)

// RequireRoot re-executes the installer as root when needed.
func RequireRoot(ctx context.Context, logger *slog.Logger, autoConfirm bool) error {
	if os.Geteuid() == 0 {
		return nil
	}

	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve installer executable: %w", err)
	}

	for _, tool := range []string{"run0", "pkexec", "sudo", "doas"} {
		toolPath, err := exec.LookPath(tool)
		if err != nil {
			continue
		}
		args := append([]string{toolPath, executablePath}, os.Args[1:]...)
		plan := enact.Plan{
			Info: "Installation requires root privileges",
			Goal: "re-execute the installer as root",
			Steps: []enact.Step{
				enact.FuncStep{
					Name: fmt.Sprintf("Execute %q", strings.Join(args, " ")),
					Fn: func(context.Context) error {
						return syscall.Exec(toolPath, args, os.Environ())
					},
				},
			},
		}
		confirmed, err := plan.Execute(ctx, logger, autoConfirm)
		if err != nil {
			return err
		}
		if !confirmed {
			return errors.New("root privileges required; run with sudo or as root")
		}
		return nil
	}
	return errors.New("root privileges required, but none of run0, pkexec, sudo or doas is available")
}
