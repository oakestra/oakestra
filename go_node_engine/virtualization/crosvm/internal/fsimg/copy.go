package fsimg

import (
	"errors"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/util/safedefer"
	"os"
	"os/exec"
	"sync"
)

var ErrMissingMountExecutable = errors.New("missing mount executable")
var ErrMissingUmountExecutable = errors.New("missing umount executable")

const mountExecutableName = "mount"
const umountExecutableName = "umount"

var mountExecutablePath = sync.OnceValue(func() string {
	return lookPathOrEmpty(mountExecutableName)
})
var umountExecutablePath = sync.OnceValue(func() string {
	return lookPathOrEmpty(umountExecutableName)
})

func CopySquashFsIntoExt4Img(squashfsPath string, ext4Path string, mountDirPath string) error {
	mountExecPath := mountExecutablePath()
	if mountExecPath == "" {
		return ErrMissingMountExecutable
	}

	umountExecPath := umountExecutablePath()
	if umountExecPath == "" {
		return ErrMissingUmountExecutable
	}

	// check both paths before calling external commands, so that callers get better errors
	// when the specified files do not exist or aren't accessible
	if _, err := os.Stat(squashfsPath); err != nil {
		return err
	}
	if _, err := os.Stat(ext4Path); err != nil {
		return err
	}

	if err := os.Mkdir(mountDirPath, 0o700); err != nil {
		return fmt.Errorf("fsimg: failed to create mount directory: %w", err)
	}
	defer safedefer.SafeDefer(
		func() error { return os.Remove(mountDirPath) },
		fmt.Sprintf("failed to remove directory %q", mountDirPath),
	)

	logger.InfoLogger().Printf("fsimg: mounting ext4 image at %q to %q", ext4Path, mountDirPath)

	mountCmd := exec.Command(mountExecPath, "-t", "ext4", "-o", "loop", ext4Path, mountDirPath)
	if err := mountCmd.Run(); err != nil {
		logger.ErrorLogger().Printf("fsimg: failed to run mount command: %v", err)
		return fmt.Errorf("fsimg: failed to run mount command: %w", err)
	}
	defer func() {
		umountCmd := exec.Command(umountExecPath, "-d", mountDirPath)
		if err := umountCmd.Run(); err != nil {
			logger.WarnLogger().Printf("fsimg: failed to run umount command: %v", err)
		}
	}()

	logger.InfoLogger().Printf("fsimg: copying squashfs contents of %q to ext4 mount at %q", squashfsPath, mountDirPath)

	if err := UnpackFromSquashFsImg(squashfsPath, mountDirPath); err != nil {
		logger.ErrorLogger().Printf("fsimg: failed to unpack squashfs image: %v", err)
		return fmt.Errorf("fsimg: failed to unpack squashfs image: %w", err)
	}

	logger.InfoLogger().Printf("fsimg: successfully copied squashfs contents of %q into ext4 image at %q", squashfsPath, ext4Path)
	return nil
}
