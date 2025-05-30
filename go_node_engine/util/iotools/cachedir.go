package iotools

import (
	"os"
)

func CreateOakestraCacheDir() (string, error) {
	basePath, err := os.UserCacheDir()
	if err != nil {
		info, err := os.Stat("/var/cache")
		if err != nil || !info.Mode().IsDir() {
			return CreateLargeTempDir("cache")
		}

		basePath = "/var/cache"
	}

	return CreateSubDir(basePath, "oakestra", 0o700)
}
