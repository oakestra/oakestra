package fsimg

import (
	"errors"
	"os/exec"
	"sync"
)

var ErrMissingMksquashfsExecutable = errors.New("missing mksquashfs executable")
var ErrMissingUnsquashfsExecutable = errors.New("missing unsquashfs executable")

const mksquashfsExecutableName = "mksquashfs"
const unsquashfsExecutableName = "unsquashfs"

var mksquashfsExecutablePath = sync.OnceValue(func() string {
	return lookPathOrEmpty(mksquashfsExecutableName)
})
var unsquashfsExecutablePath = sync.OnceValue(func() string {
	return lookPathOrEmpty(unsquashfsExecutableName)
})

func PackIntoSquashFsImg(srcDirPath string, dstPath string) error {
	execPath := mksquashfsExecutablePath()
	if execPath == "" {
		return ErrMissingMksquashfsExecutable
	}

	cmd := exec.Command(execPath, srcDirPath, dstPath)
	return cmd.Run()
}

func UnpackFromSquashFsImg(srcPath string, dstDirPath string) error {
	execPath := unsquashfsExecutablePath()
	if execPath == "" {
		return ErrMissingUnsquashfsExecutable
	}

	cmd := exec.Command(execPath, "-d", dstDirPath, srcPath)
	return cmd.Run()
}
