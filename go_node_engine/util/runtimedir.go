package util

import (
	"os"
	"path"
)

func CreateBaseRuntimeDir() (string, error) {
	xdgPath := os.Getenv("XDG_RUNTIME_DIR")
	if xdgPath == "" {
		return os.MkdirTemp("", "*-oakestra")
	}

	basePath := path.Join(xdgPath, "oakestra")
	err := os.MkdirAll(basePath, 0700)
	if err != nil {
		return "", err
	}

	return basePath, nil
}

func CreateSubRuntimeDir(baseRuntimeDirPath string, name string) (string, error) {
	subPath := path.Join(baseRuntimeDirPath, name)
	if err := os.MkdirAll(subPath, 0700); err != nil {
		return "", err
	}

	return subPath, nil
}
