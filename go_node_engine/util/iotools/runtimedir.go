package iotools

import (
	"os"
	"path"
)

func CreateOakestraRuntimeDir() (string, error) {
	basePath := os.Getenv("XDG_RUNTIME_DIR")
	if basePath == "" {
		return os.MkdirTemp("", "*-oakestra")
	}

	oakestraPath := path.Join(basePath, "oakestra")
	err := os.MkdirAll(oakestraPath, 0o700)
	if err != nil {
		return "", err
	}

	return oakestraPath, nil
}
