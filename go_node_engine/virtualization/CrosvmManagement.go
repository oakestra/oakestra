package virtualization

// #cgo LDFLAGS: ${SRCDIR}/../third_party/crosvm/crosvm/target/release/libcrosvm_control.a
// #include "../third_party/crosvm/crosvm/target/release/crosvm_control.h"
import "C"
import (
	"go_node_engine/logger"
	"go_node_engine/model"
	"os/exec"
	"sync"
)

const CrosvmExecutableName = "crosvm"

type CrosvmRuntime struct {
	crosvmExecutablePath string
}

type CrosvmAction int

const (
	CrosvmActionUndeploy CrosvmAction = iota
)

type CrosvmInstance struct {
	actionQueue chan CrosvmAction
}

var crosvmRuntime = CrosvmRuntime{}

var crosvmSyncOnce sync.Once

func GetCrosvmRuntime() *CrosvmRuntime {
	crosvmSyncOnce.Do(func() {
		path, err := exec.LookPath(CrosvmExecutableName)
		if err != nil {
			logger.ErrorLogger().Printf("Unable to find crosvm executable(%s): %v\n", CrosvmExecutableName, err)
			crosvmRuntime.crosvmExecutablePath = ""
			return
		}

		crosvmRuntime.crosvmExecutablePath = path
		logger.InfoLogger().Printf("Using crosvm at %s\n", path)

		model.GetNodeInfo().AddSupportedTechnology(model.CROSVM_RUNTIME)
	})
	return &crosvmRuntime
}

func (r *CrosvmRuntime) Deploy(service model.Service, statusChangeNotificationHandler func(service model.Service)) error {
	return nil
}

func (r *CrosvmRuntime) Undeploy(sname string, instance int) error {
	return nil
}

func (r *CrosvmRuntime) Stop() {

}

func (r *CrosvmRuntime) crosvmInstanceRoutine(instance *CrosvmInstance) {
	err := exec.Command(r.crosvmExecutablePath, kernel_location+"files", instance_path).Run()
	if err != nil {
		logger.InfoLogger().Printf("Unable to set files: %v", err)
	}

	select {
	case action := <-i.actionQueue:
		println(action)
	}
}
