package install

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"charm.land/huh/v2"
	"github.com/oakestra/oakestra/installer/util/download"
	"github.com/oakestra/oakestra/installer/util/iotools"
	"github.com/oakestra/oakestra/installer/util/prompt"
)

const netManagerConfigDir = "/etc/netmanager"

func (i installer) installFirstParty(ctx context.Context, version string) (bool, error) {
	var nodeArchivePath, netArchivePath string
	source := "remote"
	if !i.autoConfirm {
		confirmed, err := selectArchives(ctx, version, &source, &nodeArchivePath, &netArchivePath)
		if err != nil || !confirmed {
			return false, err
		}
	}

	for _, dir := range []string{"/bin", "/etc/systemd/system", netManagerConfigDir, "/var/log/oakestra"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return false, fmt.Errorf("create worker directory %q: %w", dir, err)
		}
	}

	if source == "remote" {
		tempDir, err := iotools.CreateLargeTempDir("worker")
		if err != nil {
			return false, err
		}
		defer iotools.RemoveAllOrWarn(i.logger, tempDir)

		nodeArchivePath, netArchivePath, err = i.downloadArchives(ctx, version, tempDir)
		if err != nil {
			return false, err
		}
	}

	i.logger.InfoContext(ctx, "Installing Oakestra worker binaries and services")
	if err := installArchives(ctx, nodeArchivePath, netArchivePath); err != nil {
		return false, err
	}
	return true, nil
}

func selectArchives(ctx context.Context, version string, source, nodeArchivePath, netArchivePath *string) (bool, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return false, fmt.Errorf("determine initial archive directory: %w", err)
	}

	releaseLabel := "Download GitHub release"
	if version != "" {
		releaseLabel += " " + version
	}
	confirmed := true
	form := huh.NewForm(
		huh.NewGroup(huh.NewSelect[string]().
			Title("Installation source").
			Options(
				huh.NewOption(releaseLabel, "remote"),
				huh.NewOption("Provide local archives", "local"),
			).
			Value(source)),
		huh.NewGroup(
			huh.NewFilePicker().
				Title("NodeEngine archive").
				AllowedTypes([]string{".tar.gz", ".tgz"}).
				Validate(huh.ValidateNotEmpty()).
				CurrentDirectory(currentDir).
				Value(nodeArchivePath),
			huh.NewFilePicker().
				Title("NetManager archive").
				AllowedTypes([]string{".tar.gz", ".tgz"}).
				Validate(huh.ValidateNotEmpty()).
				CurrentDirectory(currentDir).
				Value(netArchivePath),
		).WithHideFunc(func() bool { return *source != "local" }),
		huh.NewGroup(huh.NewConfirm().
			Title("Proceed with installation?").
			Value(&confirmed)),
	)
	if err := prompt.RunForm(ctx, form); err != nil {
		return false, fmt.Errorf("select worker archives: %w", err)
	}
	return confirmed, nil
}

func resolveOakestraVersion(ctx context.Context, version string) (string, error) {
	const baseURL = "https://raw.githubusercontent.com/oakestra/oakestra/"
	var ref, prefix string
	switch version {
	case "":
		ref = "main"
	case "alpha":
		ref, prefix = "develop", "alpha-"
	default:
		return version, nil
	}

	release, err := download.GetText(ctx, baseURL+ref+"/version.txt")
	if err != nil {
		return "", err
	}
	if release == "" {
		return "", errors.New("Oakestra version file is empty")
	}
	return prefix + release, nil
}

func (i installer) downloadArchives(ctx context.Context, version, tempDir string) (string, string, error) {
	resolvedVersion, err := resolveOakestraVersion(ctx, version)
	if err != nil {
		return "", "", fmt.Errorf("resolve Oakestra version: %w", err)
	}
	tag := url.PathEscape(resolvedVersion)
	nodeArchivePath := filepath.Join(tempDir, "NodeEngine.tar.gz")
	netArchivePath := filepath.Join(tempDir, "NetManager.tar.gz")
	nodeURL := fmt.Sprintf("https://github.com/oakestra/oakestra/releases/download/%s/NodeEngine_%s.tar.gz", tag, runtime.GOARCH)
	netURL := fmt.Sprintf("https://github.com/oakestra/oakestra-net/releases/download/%s/NetManager_%s.tar.gz", tag, runtime.GOARCH)

	i.logger.InfoContext(ctx, "Downloading NodeEngine", "version", resolvedVersion)
	if err := download.GetFile(ctx, nodeURL, nodeArchivePath, 0o644); err != nil {
		return "", "", fmt.Errorf("download NodeEngine: %w", err)
	}
	i.logger.InfoContext(ctx, "Downloading NetManager", "version", resolvedVersion)
	if err := download.GetFile(ctx, netURL, netArchivePath, 0o644); err != nil {
		return "", "", fmt.Errorf("download NetManager: %w", err)
	}
	return nodeArchivePath, netArchivePath, nil
}

func installArchives(ctx context.Context, nodeArchivePath, netArchivePath string) (err error) {
	configTargets, err := netManagerConfigTargets()
	if err != nil {
		return err
	}
	staged := iotools.NewStagedFiles()
	defer func() { err = errors.Join(err, staged.Close()) }()

	if err := staged.StageTarGzip(ctx, nodeArchivePath,
		iotools.ExactTarTarget{TarName: "NodeEngine", DstPath: "/bin/NodeEngine", DstPerm: 0o755},
		iotools.ExactTarTarget{TarName: "nodeengined", DstPath: "/bin/nodeengined", DstPerm: 0o755},
		iotools.ExactTarTarget{TarName: "nodeengine.service", DstPath: "/etc/systemd/system/nodeengine.service", DstPerm: 0o644},
	); err != nil {
		return err
	}
	netTargets := append([]iotools.TarTarget{
		iotools.ExactTarTarget{TarName: "NetManager", DstPath: "/bin/NetManager", DstPerm: 0o755},
		iotools.ExactTarTarget{TarName: "netmanager.service", DstPath: "/etc/systemd/system/netmanager.service", DstPerm: 0o644},
	}, configTargets...)
	if err := staged.StageTarGzip(ctx, netArchivePath, netTargets...); err != nil {
		return err
	}

	if err := stopSystemdUnits(ctx); err != nil {
		return err
	}
	if err := staged.Commit(ctx); err != nil {
		return err
	}
	return reloadSystemdUnits(ctx)
}

func netManagerConfigTargets() ([]iotools.TarTarget, error) {
	names := []string{"tuncfg.json", "netcfg.json"}
	targets := make([]iotools.TarTarget, 0, len(names))
	for _, name := range names {
		dstPath := filepath.Join(netManagerConfigDir, name)
		if _, err := os.Lstat(dstPath); err == nil {
			dstPath += ".new"
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("check NetManager config %q: %w", dstPath, err)
		}
		targets = append(targets, iotools.ExactTarTarget{TarName: name, DstPath: dstPath, DstPerm: 0o644})
	}
	return targets, nil
}
