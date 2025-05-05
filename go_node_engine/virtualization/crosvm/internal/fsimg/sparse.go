package fsimg

import (
	"go_node_engine/virtualization/crosvm/internal/safedefer"
	"os"
	"syscall"
)

func CreateSparseFile(dstPath string, perm os.FileMode, size int64) error {
	dstFile, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer safedefer.SafeClose(dstFile, dstPath)

	return ftruncate(dstFile, size)
}

func ftruncate(file *os.File, length int64) error {
	syscallConn, err := file.SyscallConn()
	if err != nil {
		return err
	}

	var truncateErr error = nil
	err = syscallConn.Control(func(fd uintptr) {
		truncateErr = ignoringEINTR(func() error {
			return syscall.Ftruncate(int(fd), length)
		})
	})
	if err != nil {
		return err
	}
	if truncateErr != nil {
		return truncateErr
	}

	return nil
}

func ignoringEINTR(fn func() error) error {
	for {
		err := fn()
		if err != syscall.EINTR {
			return err
		}
	}
}
