package virtualization

import (
	"go_node_engine/model"
	"time"
)

type RuntimeInterface interface {
	Deploy(service model.Service, statusChangeNotificationHandler func(service model.Service)) error
	Undeploy(sname string, instance int) error
	Stop()
}

type RuntimeMonitoring interface {
	ResourceMonitoring(every time.Duration, notifyHandler func(res []model.Resources))
}

// RuntimeMigration defines the interface for runtimes that support migration.
// It includes methods to check if a service can be migrated, stop a service and get its state,
// IMPORTANT: This runtime interface is OPTIONAL.
// Runtimes that do not support statefull migration should not implement this interface.
type RuntimeMigration interface {
	CanBeMigrated(sname string, instance int) bool
	StopAndGetState(sname string, instance int) ([]byte, error)
	PrepareForInstantiantion(service model.Service, statusChangeNotificationHandler func(service model.Service)) error
	ResumeFromState(sname string, instance int, state []byte) error
}

type Runtime interface {
	RuntimeInterface
	RuntimeMonitoring
}

type RuntimeType string
type RuntimeGetter func() Runtime

var runtimeMap = map[model.RuntimeType]RuntimeGetter{}

func init() {
	runtimeMap[model.CONTAINER_RUNTIME] = RuntimeGetter(GetContainerdRuntime)
	runtimeMap[model.UNIKERNEL_RUNTIME] = RuntimeGetter(GetUnikernelQemuRuntime)
}

func GetRuntime(runtime model.RuntimeType) RuntimeInterface {
	return runtimeMap[runtime]()
}

func GetRuntimeMigration(runtime model.RuntimeType) (RuntimeMigration, error) {
	if r, ok := runtimeMap[runtime]; ok {
		if rm, ok := r().(RuntimeMigration); ok {
			return rm, nil
		}
	}
	return nil, model.ErrRuntimeMigrationNotSupported
}

func GetRuntimeMonitoring(runtime model.RuntimeType) RuntimeMonitoring {
	return runtimeMap[runtime]()
}

// can be used by registered runtimes to register additional sub-runtimes.
// E.g. containerd can register runc as well as urunc
func registerRuntimeLink(name string, getter RuntimeGetter) {
	runtimeMap[model.RuntimeType(name)] = getter
}
