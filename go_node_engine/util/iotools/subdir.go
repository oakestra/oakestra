package iotools

import (
	"github.com/spf13/afero"
	"os"
	"path"
)

func CreateSubDir(basePath string, subName string, perm os.FileMode) (string, error) {
	return CreateSubDirInFs(afero.NewOsFs(), basePath, subName, perm)
}

func CreateSubDirInFs(fs afero.Fs, basePath string, subName string, perm os.FileMode) (string, error) {
	subPath := path.Join(basePath, subName)
	if err := fs.MkdirAll(subPath, perm); err != nil {
		return "", err
	}

	return subPath, nil
}
