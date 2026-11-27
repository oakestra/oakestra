package install

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/oakestra/oakestra/installer/util/command"
	"github.com/oakestra/oakestra/installer/util/download"
	"github.com/oakestra/oakestra/installer/util/enact"
	"github.com/oakestra/oakestra/installer/util/iotools"
)

//go:embed cni-default.conflist
var cniDefaultConfig string

//go:embed crictl-default.yaml
var crictlDefaultConfig string

func (i installer) installContainerd(ctx context.Context) (bool, error) {
	if err := command.RunSilent(ctx, "systemctl", "is-active", "--quiet", "containerd"); err == nil {
		i.logger.InfoContext(ctx, "Existing containerd installation is already running.")
		return true, nil
	} else if command.IsContextError(err) {
		return false, err
	}

	installed, err := checkIsContainerdInstalled(ctx)
	if err != nil {
		return false, err
	}
	if installed {
		plan := &enact.Plan{
			Info:  "Existing containerd installation is not running",
			Goal:  "start and enable it",
			Steps: getContainerdEnableSteps(),
		}
		return plan.Execute(ctx, i.logger, i.autoConfirm)
	}

	tempDir, err := iotools.CreateLargeTempDir("containerd-install")
	if err != nil {
		return false, err
	}
	defer iotools.RemoveAllOrWarn(i.logger, tempDir)

	plan := &enact.Plan{
		Info:  "No existing installation of containerd detected",
		Goal:  "download and install containerd stack",
		Steps: getCustomContainerdInstallSteps(tempDir),
	}
	return plan.Execute(ctx, i.logger, i.autoConfirm)
}

func checkIsContainerdInstalled(ctx context.Context) (bool, error) {
	output, err := command.RunCaptured(ctx, "systemctl", "list-unit-files", "containerd.service")
	if command.IsContextError(err) {
		return false, err
	}

	// on some versions, systemd errors for non-existent units
	if err != nil {
		return false, nil
	}

	// on others, it succeeds but prints "0 unit files listed"
	return !strings.Contains(string(output), "0 unit files listed."), nil
}

func getContainerdEnableSteps() []enact.Step {
	return []enact.Step{
		enact.CommandStep{
			Command: "systemctl",
			Args:    []string{"enable", "--now", "containerd"},
		},
	}
}

func getCustomContainerdInstallSteps(tempDir string) []enact.Step {
	return []enact.Step{
		enact.FuncStep{
			Name: "Download and install runc to /usr/local/sbin/",
			Fn: func(ctx context.Context) error {
				if err := os.MkdirAll("/usr/local/sbin", 0o755); err != nil {
					return err
				}
				url := fmt.Sprintf("https://github.com/opencontainers/runc/releases/download/v1.5.0/runc.%s", runtime.GOARCH)
				return download.GetFile(ctx, url, "/usr/local/sbin/runc", 0o755)
			},
		},
		enact.FuncStep{
			Name: "Install CNI plugins and crictl with their configuration, then install the containerd binaries",
			Fn: func(ctx context.Context) error {
				return installRuntimeArchives(ctx, tempDir)
			},
		},
		enact.FuncStep{
			Name: "Generate /etc/containerd/config.toml and install the containerd service",
			Fn: func(ctx context.Context) error {
				if err := os.MkdirAll("/usr/local/lib/systemd/system", 0o755); err != nil {
					return err
				}
				if err := os.MkdirAll("/etc/containerd", 0o755); err != nil {
					return err
				}
				out, err := command.RunCaptured(ctx, "/usr/local/bin/containerd", "config", "default")
				if err != nil {
					return fmt.Errorf("failed to generate default containerd config: %w", err)
				}

				if err := iotools.WriteFileAtomic(ctx, "/etc/containerd/config.toml", 0o644, bytes.NewReader(out)); err != nil {
					return err
				}
				return download.GetFile(ctx, "https://raw.githubusercontent.com/containerd/containerd/refs/tags/v2.3.2/containerd.service", "/usr/local/lib/systemd/system/containerd.service", 0o644)
			},
		},
		enact.CommandStep{
			Command: "systemctl",
			Args:    []string{"daemon-reload"},
		},
		enact.CommandStep{
			Command: "systemctl",
			Args:    []string{"enable", "--now", "containerd"},
		},
	}
}

func installRuntimeArchives(ctx context.Context, tempDir string) (err error) {
	const binDir = "/usr/local/bin"
	const cniBinDir = "/opt/cni/bin"
	const cniConfigPath = "/etc/cni/net.d/10-containerd-net.conflist"
	const crictlConfigPath = "/etc/crictl.yaml"
	for _, dir := range []string{binDir, cniBinDir, filepath.Dir(cniConfigPath), filepath.Dir(crictlConfigPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create runtime directory %q: %w", dir, err)
		}
	}
	staged := iotools.NewStagedFiles()
	defer func() { err = errors.Join(err, staged.Close()) }()

	topLevelFiles := regexp.MustCompile(`^([^/]+)$`)
	cniURL := fmt.Sprintf("https://github.com/containernetworking/plugins/releases/download/v1.9.1/cni-plugins-linux-%s-v1.9.1.tgz", runtime.GOARCH)
	if err := downloadAndStageRuntimeArchive(ctx, staged, cniURL, filepath.Join(tempDir, "cni-plugins.tgz"), cniBinDir, topLevelFiles); err != nil {
		return err
	}
	if err := staged.Prepare(ctx, cniConfigPath, 0o644, strings.NewReader(cniDefaultConfig)); err != nil {
		return err
	}

	crictlURL := fmt.Sprintf("https://github.com/kubernetes-sigs/cri-tools/releases/download/v1.36.0/crictl-v1.36.0-linux-%s.tar.gz", runtime.GOARCH)
	if err := downloadAndStageRuntimeArchive(ctx, staged, crictlURL, filepath.Join(tempDir, "crictl.tar.gz"), binDir, topLevelFiles); err != nil {
		return err
	}
	if err := staged.Prepare(ctx, crictlConfigPath, 0o644, strings.NewReader(crictlDefaultConfig)); err != nil {
		return err
	}

	containerdURL := fmt.Sprintf("https://github.com/containerd/containerd/releases/download/v2.3.2/containerd-2.3.2-linux-%s.tar.gz", runtime.GOARCH)
	if err := downloadAndStageRuntimeArchive(ctx, staged, containerdURL, filepath.Join(tempDir, "containerd.tar.gz"),
		binDir, regexp.MustCompile(`^bin/([^/]+)$`),
	); err != nil {
		return err
	}

	return staged.Commit(ctx)
}

func downloadAndStageRuntimeArchive(ctx context.Context, staged *iotools.StagedFiles, url, archivePath, dstDir string, tarNameRegex *regexp.Regexp) error {
	if err := download.GetFile(ctx, url, archivePath, 0o644); err != nil {
		return err
	}
	return staged.StageTarGzip(ctx, archivePath, iotools.RegexpTarTarget{
		TarNameRegex:    tarNameRegex,
		DstPathTemplate: filepath.Join(dstDir, "$1"),
		DstPerm:         0o755,
	})
}
