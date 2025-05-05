package dirutil

import (
	"fmt"
	"github.com/spf13/afero"
	"os"
	"sync"
)

var largeTempDirBase = sync.OnceValue(func() string {
	varTmpInfo, err := os.Stat("/var/tmp")
	if err != nil || !varTmpInfo.IsDir() {
		return ""
	}

	return "/var/tmp"
})

func CreateTempDir(tag string) (string, error) {
	return CreateTempDirInFs(afero.NewOsFs(), tag)
}

func CreateTempDirInFs(fs afero.Fs, tag string) (string, error) {
	return afero.TempDir(fs, "", fmt.Sprintf("oakestra-%s-", tag))
}

func CreateLargeTempDir(tag string) (string, error) {
	return CreateLargeTempDirInFs(afero.NewOsFs(), tag)
}

func CreateLargeTempDirInFs(fs afero.Fs, tag string) (string, error) {
	if fs.Name() != "OsFs" {
		return CreateTempDirInFs(fs, tag)
	}

	return afero.TempDir(afero.NewOsFs(), largeTempDirBase(), fmt.Sprintf("oakestra-%s-", tag))
}
