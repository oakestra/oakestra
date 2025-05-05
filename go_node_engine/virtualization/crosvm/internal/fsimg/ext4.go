package fsimg

import (
	"errors"
	"os"
	"os/exec"
	"sync"
)

var ErrMissingMkfsExt4Executable = errors.New("missing mkfs.ext4 executable")

const mkfsExt4ExecutableName = "mkfs.ext4"

var mkfsExt4ExecutablePath = sync.OnceValue(func() string {
	return lookPathOrEmpty(mkfsExt4ExecutableName)
})

func CreateExt4Img(dstPath string, perm os.FileMode, size int64) error {
	if err := CreateSparseFile(dstPath, perm, size); err != nil {
		return err
	}

	execPath := mkfsExt4ExecutablePath()
	if execPath == "" {
		return ErrMissingMkfsExt4Executable
	}

	cmd := exec.Command(execPath, dstPath)
	return cmd.Run()
}
