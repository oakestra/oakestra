package iotools

import (
	"os"
)

func CreateOakestraStateDir() (string, error) {
	basePath := os.Getenv("XDG_STATE_HOME")
	if basePath == "" {
		info, err := os.Stat("/var/lib")
		if err != nil || !info.Mode().IsDir() {
			return CreateLargeTempDir("state")
		}

		basePath = "/var/lib"
	}

	return CreateSubDir(basePath, "oakestra", 0o700)
}
