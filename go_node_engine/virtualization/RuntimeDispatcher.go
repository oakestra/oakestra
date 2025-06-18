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
	// SetMigrationCandidate checks if the service can be migrated and marks it as a candidate.
	// It returns an error if the service is not migratable or if it is already marked as a candidate.
	SetMigrationCandidate(sname string, instance int) (model.Service, error)
	// RemoveMigrationCandidate removes the migration candidate mark from a service.
	// It returns an error if the service is not marked as a candidate or if it does not exist.
	RemoveMigrationCandidate(sname string, instance int) error
	// StopAndGetState stops a service and returns its state if it has been marked as a migration candidate.
	// The state is a byte slice that can be used to resume the service later.
	StopAndGetState(sname string, instance int) ([]byte, error)
	// PrepareForInstantiantion prepares the service for instantiation after mgiration. The state is not ready
	// to be resumed yet, but the service code can be donwloaded and the virtualization's environment prepared.
	PrepareForInstantiantion(service model.Service, statusChangeNotificationHandler func(service model.Service)) error
	// AbortMigration aborts the migration process for a service initiated by PrepareForInstantiantion.
	// It should be called if the migration is not going to be completed, e.g. if the service is not going to be resumed.
	AbortMigration(service model.Service) error
	// ResumeFromState resumes a service prepared for instantiation with a given state.
	// The state is a byte slice that was returned by StopAndGetState method.
	// This method can be called only after PrepareForInstantiantion returned without an error.
	ResumeFromState(sname string, instance int, state []byte, statusChangeNotificationHandler func(service model.Service)) error
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
