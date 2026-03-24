package iotools

import (
	"os"
)

func CreateOakestraRuntimeDir() (string, error) {
	basePath := os.Getenv("XDG_RUNTIME_DIR")
	if basePath == "" {
		info, err := os.Stat("/run")
		if err != nil || !info.Mode().IsDir() {
			return CreateTempDir("runtime")
		}

		basePath = "/run"
	}

	return CreateSubDir(basePath, "oakestra", 0o700)
}
