package iotools

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

// WriteFileAtomic writes a complete replacement and installs it with a rename.
func WriteFileAtomic(ctx context.Context, dstPath string, perm os.FileMode, reader io.Reader) error {
	return WriteFileAtomicInFs(ctx, afero.NewOsFs(), dstPath, perm, reader)
}

func WriteFileAtomicInFs(ctx context.Context, fs afero.Fs, dstPath string, perm os.FileMode, reader io.Reader) (err error) {
	staged := NewStagedFilesInFs(fs)
	defer func() { err = errors.Join(err, staged.Close()) }()

	if err := staged.Prepare(ctx, dstPath, perm, reader); err != nil {
		return err
	}
	return staged.Commit(ctx)
}

func prepareFileInFs(ctx context.Context, fs afero.Fs, dstPath string, perm os.FileMode, reader io.Reader) (_ string, err error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	file, err := afero.TempFile(fs, filepath.Dir(dstPath), ".oakestra-")
	if err != nil {
		return "", err
	}
	tempPath := file.Name()
	defer func() {
		err = errors.Join(err, file.Close())
		if err != nil {
			err = errors.Join(err, fs.Remove(tempPath))
		}
	}()

	if _, err := io.Copy(file, contextReader{ctx: ctx, reader: reader}); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := fs.Chmod(tempPath, perm); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	return tempPath, ctx.Err()
}
