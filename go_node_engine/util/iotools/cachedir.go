package iotools

import (
	"os"
	"path"
)

func CreateOakestraCacheDir() (string, error) {
	basePath, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	oakestraPath := path.Join(basePath, "oakestra")
	if err := os.MkdirAll(oakestraPath, 0o700); err != nil {
		return "", err
	}

	return oakestraPath, nil
}
