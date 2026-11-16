package fsimg

import (
	"errors"
	"os/exec"
	"sync"
)

var ErrMissingMkisofsExecutable = errors.New("missing mkisofs executable")

const mkisofsExecutableName = "mkisofs"

var mkisofsExecutablePath = sync.OnceValue(func() string {
	return lookPathOrEmpty(mkisofsExecutableName)
})

func PackIntoIsoFsImg(volumeId string, srcDirPath string, dstPath string) error {
	execPath := mkisofsExecutablePath()
	if execPath == "" {
		return ErrMissingMkisofsExecutable
	}

	cmd := exec.Command(
		execPath,
		"-o",
		dstPath,
		"-V",
		volumeId,
		"-J", // enable joliet extension
		"-R", // enable rock ridge extension
		srcDirPath,
	)
	return cmd.Run()
}
