package iotools

import "github.com/spf13/afero"

func CreateLargeTempDir(tag string) (string, error) {
	return CreateLargeTempDirInFs(afero.NewOsFs(), tag)
}

func CreateLargeTempDirInFs(fs afero.Fs, tag string) (string, error) {
	baseDir := ""
	if info, err := fs.Stat("/var/tmp"); err == nil && info.IsDir() {
		baseDir = "/var/tmp"
	}
	return afero.TempDir(fs, baseDir, "oakestra-"+tag+"-")
}
