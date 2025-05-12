package image

import (
	"os"
	"path"
	"strings"
)

type Image struct {
	Key       string
	Size      int64
	HasInitrd bool
}

func CreateImageFromDir(imageDirPath string) (*Image, error) {
	key := strings.TrimSuffix(imageDirPath, imageDirExtension)

	kernelInfo, err := os.Stat(path.Join(imageDirPath, KernelFileName))
	if err != nil {
		return nil, err
	}
	kernelSize := kernelInfo.Size()

	initrdInfo, err := os.Stat(path.Join(imageDirPath, InitrdFileName))
	// initrd is optional so it's fine if it doesn't exist, while all other errors indicate a bigger problem
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var initrdSize int64 = 0
	if initrdInfo != nil {
		initrdSize = initrdInfo.Size()
	}

	rootfsInfo, err := os.Stat(path.Join(imageDirPath, internalRootfsFileName))
	if err != nil {
		return nil, err
	}
	rootfsSize := rootfsInfo.Size()

	return &Image{
		Key:       key,
		Size:      kernelSize + initrdSize + rootfsSize,
		HasInitrd: initrdSize > 0,
	}, nil
}
