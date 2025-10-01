package image_test

import (
	"go_node_engine/util/iotools"
	"go_node_engine/virtualization/internal/crosvm/internal/image"
	"os"
	"testing"

	"github.com/containers/image/v5/docker"
	"github.com/containers/storage/pkg/reexec"
	"gotest.tools/v3/assert"
)

func init() {
	reexec.Init()
}

func TestFetchImage(t *testing.T) {
	tmpDirPath, err := iotools.CreateLargeTempDir("image-store-test")
	assert.NilError(t, err)
	defer func() { assert.NilError(t, os.RemoveAll(tmpDirPath)) }()

	store, err := image.NewStore(tmpDirPath, image.NewContainersSource(docker.Transport))
	assert.NilError(t, err)

	img, err := store.Retrieve("//alpine:3.21.3", tmpDirPath, 2<<30)
	assert.NilError(t, err)
	assert.Assert(t, img != nil)
}
