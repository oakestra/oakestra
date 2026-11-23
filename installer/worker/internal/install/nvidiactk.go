package install

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/oakestra/oakestra/installer/util/command"
	"github.com/oakestra/oakestra/installer/util/download"
	"github.com/oakestra/oakestra/installer/util/enact"
	"github.com/oakestra/oakestra/installer/util/iotools"
)

func (i installer) installNvidiaCTK(ctx context.Context, osFamily OSFamily) error {
	// nvidia-smi for standard GPUs, tegrastats for Jetson GPUs
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		if _, err := exec.LookPath("tegrastats"); err != nil {
			i.logger.InfoContext(ctx, "No NVIDIA GPU detected, skipping NVIDIA Container Toolkit installation.")
			return nil
		}
	}

	if _, err := exec.LookPath("nvidia-ctk"); err == nil {
		plan := &enact.Plan{
			Info:  "Detected existing installation of NVIDIA Container Toolkit",
			Goal:  "configure it",
			Steps: getNvidiaCTKConfigureCommands(),
		}

		_, err := plan.Execute(ctx, i.logger, i.autoConfirm)
		return err
	}

	if ready, err := i.ensureNvidiaCTKPrerequisites(ctx, osFamily); err != nil || !ready {
		return err
	}

	installSteps := getNvidiaCTKInstallCommands(osFamily)
	if installSteps == nil {
		i.logger.InfoContext(ctx, "Automatic installation of NVIDIA Container Toolkit not supported for the current OS, skipping.")
		return nil
	}

	plan := &enact.Plan{
		Info:  "No existing installation of NVIDIA Container Toolkit was detected",
		Goal:  "install it",
		Steps: installSteps,
	}

	_, err := plan.Execute(ctx, i.logger, i.autoConfirm)
	return err
}

func (i installer) ensureNvidiaCTKPrerequisites(ctx context.Context, osFamily OSFamily) (bool, error) {
	if osFamily != Apt {
		return true, nil
	}

	if _, err := exec.LookPath("gpg"); err == nil {
		return true, nil
	}

	plan := &enact.Plan{
		Info: "Missing gpg command",
		Goal: "install it",
		Steps: []enact.Step{
			enact.CommandStep{
				Command: "apt-get",
				Args:    []string{"update"},
			},
			enact.CommandStep{
				Command: "apt-get",
				Args:    []string{"install", "-y", "gnupg"},
			},
		},
	}

	return plan.Execute(ctx, i.logger, i.autoConfirm)
}

func getNvidiaCTKConfigureCommands() []enact.Step {
	return []enact.Step{
		enact.CommandStep{
			Command: "nvidia-ctk",
			Args:    []string{"runtime", "configure", "--runtime=containerd"},
		},
		enact.CommandStep{
			Command: "nvidia-ctk",
			Args:    []string{"cdi", "generate", "--output=/etc/cdi/nvidia.yaml"},
		},
		enact.CommandStep{
			Command: "systemctl",
			Args:    []string{"restart", "containerd"},
		},
	}
}

func getNvidiaCTKInstallCommands(osFamily OSFamily) []enact.Step {
	var steps []enact.Step
	switch osFamily {
	case Apt:
		steps = []enact.Step{
			enact.FuncStep{
				Name: "Configure NVIDIA APT repository and signing key",
				Fn:   configureNvidiaAPT,
			},
			enact.CommandStep{Command: "apt-get", Args: []string{"update"}},
			enact.CommandStep{Command: "apt-get", Args: []string{"install", "-y", "nvidia-container-toolkit"}},
		}
	case Dnf:
		steps = []enact.Step{
			enact.FuncStep{
				Name: "Download NVIDIA Toolkit repository",
				Fn: func(ctx context.Context) error {
					const repoPath = "/etc/yum.repos.d/nvidia-container-toolkit.repo"
					if err := os.MkdirAll(filepath.Dir(repoPath), 0o755); err != nil {
						return err
					}
					return download.GetFile(ctx,
						"https://nvidia.github.io/libnvidia-container/stable/rpm/nvidia-container-toolkit.repo",
						repoPath,
						0o644,
					)
				},
			},
			enact.CommandStep{Command: "dnf", Args: []string{"install", "-y", "nvidia-container-toolkit"}},
		}
	default:
		return nil
	}
	return append(steps, getNvidiaCTKConfigureCommands()...)
}

func configureNvidiaAPT(ctx context.Context) (err error) {
	const keyringPath = "/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg"
	const repoListPath = "/etc/apt/sources.list.d/nvidia-container-toolkit.list"

	for _, dir := range []string{filepath.Dir(keyringPath), filepath.Dir(repoListPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	publicKey, err := download.GetBytes(ctx, "https://nvidia.github.io/libnvidia-container/gpgkey")
	if err != nil {
		return err
	}
	keyring, err := command.RunCapturedWithInput(ctx, bytes.NewReader(publicKey), "gpg", "--batch", "--dearmor", "--output", "-")
	if err != nil {
		return fmt.Errorf("dearmor NVIDIA repository key: %w", err)
	}

	content, err := download.GetText(ctx, "https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list")
	if err != nil {
		return err
	}
	content = strings.ReplaceAll(content, "deb https://", "deb [signed-by="+keyringPath+"] https://")
	staged := iotools.NewStagedFiles()
	defer func() { err = errors.Join(err, staged.Close()) }()
	if err := staged.Prepare(ctx, keyringPath, 0o644, bytes.NewReader(keyring)); err != nil {
		return err
	}
	if err := staged.Prepare(ctx, repoListPath, 0o644, strings.NewReader(content+"\n")); err != nil {
		return err
	}
	return staged.Commit(ctx)
}
