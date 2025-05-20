package instance_test

import (
	"github.com/containers/image/v5/oci/archive"
	"github.com/containers/storage/pkg/reexec"
	"go_node_engine/model"
	"go_node_engine/util/iotools"
	"go_node_engine/virtualization/internal/crosvm/internal/image"
	"go_node_engine/virtualization/internal/crosvm/internal/instance"
	"gotest.tools/v3/assert"
	"os"
	"path"
	"path/filepath"
	"testing"
)

func init() {
	reexec.Init()
}

func TestFetchImage(t *testing.T) {
	workDirPath, err := os.Getwd()
	assert.NilError(t, err)

	imageArchivePath, err := filepath.Abs(path.Join(workDirPath, "ssh-img.oci"))
	assert.NilError(t, err)

	storeDirPath, err := filepath.Abs(path.Join(workDirPath, "store"))
	assert.NilError(t, err)

	store, err := image.NewStore(storeDirPath, image.NewContainersSource(archive.Transport))
	assert.NilError(t, err)

	runtimeDirPath, err := iotools.CreateLargeTempDir("instance-test-runtime")
	assert.NilError(t, err)
	defer func() { assert.NilError(t, os.RemoveAll(runtimeDirPath)) }()

	inst, err := instance.NewInstance(
		"test.test.test.test",
		model.Service{
			JobID:           "test.test.test.test",
			Sname:           "test",
			Instance:        0,
			Image:           "containers-oci-archive:" + imageArchivePath,
			Commands:        nil,
			Env:             nil,
			Ports:           "",
			Status:          "",
			Runtime:         "",
			Platform:        "",
			StatusDetail:    "",
			Vtpus:           8192,
			Vgpus:           0,
			Vcpus:           2,
			Memory:          2048,
			UnikernelImages: nil,
			Architectures:   nil,
			Pid:             0,
			OneShot:         true,
			Privileged:      false,
		},
		func(_ model.Service) {},
		"/opt/oakestra/bin/crosvm",
		runtimeDirPath,
		store,
	)
	assert.NilError(t, err)

	assert.NilError(t, inst.Start())
	assert.NilError(t, inst.WaitForExit(-1))
	assert.NilError(t, inst.Close())
}
