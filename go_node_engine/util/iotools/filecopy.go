package iotools

import (
	"github.com/spf13/afero"
	"io"
	"os"
)

func CopyFile(srcPath string, dstPath string, dstPerm os.FileMode) error {
	return CopyFileInFs(afero.NewOsFs(), srcPath, dstPath, dstPerm)
}

func CopyFileInFs(fs afero.Fs, srcPath string, dstPath string, dstPerm os.FileMode) error {
	srcFile, err := fs.OpenFile(srcPath, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer CloseOrWarn(srcFile, srcPath)

	dstFile, err := fs.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, dstPerm)
	if err != nil {
		return err
	}
	defer CloseOrWarn(dstFile, dstPath)

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return nil
}
