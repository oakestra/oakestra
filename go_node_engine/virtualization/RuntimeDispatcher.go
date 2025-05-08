package virtualization

import (
	"go_node_engine/model"
	"go_node_engine/util/iotools"
	"go_node_engine/virtualization/crosvm"
	"time"
)

type Runtime interface {
	RuntimeInterface
	RuntimeMonitoring
}

type RuntimeGetter func() Runtime

type RuntimeManager struct {
	runtimeDirPath string
	cacheDirPath   string
	runtimeMap     map[model.RuntimeType]RuntimeGetter
}

type RuntimeInterface interface {
	Deploy(service model.Service, statusChangeNotificationHandler func(service model.Service)) error
	Undeploy(sname string, instance int) error
	Stop()
}

type RuntimeMonitoring interface {
	ResourceMonitoring(every time.Duration, notifyHandler func(res []model.Resources))
}

func NewRuntimeManager() (*RuntimeManager, error) {
	runtimeDirPath, err := iotools.CreateOakestraRuntimeDir()
	if err != nil {
		return nil, err
	}

	cacheDirPath, err := iotools.CreateOakestraCacheDir()
	if err != nil {
		return nil, err
	}

	var runtimeMap = make(map[model.RuntimeType]RuntimeGetter)
	runtimeMap[model.CONTAINER_RUNTIME] = GetContainerdRuntime
	runtimeMap[model.UNIKERNEL_RUNTIME] = GetUnikernelQemuRuntime
	runtimeMap[model.CROSVM_RUNTIME] = func() Runtime { return crosvm.RuntimeSingleton(runtimeDirPath, cacheDirPath) }

	return &RuntimeManager{
		runtimeDirPath: runtimeDirPath,
		cacheDirPath:   cacheDirPath,
		runtimeMap:     runtimeMap,
	}, nil
}

func (m *RuntimeManager) GetRuntime(runtime model.RuntimeType) RuntimeInterface {
	return m.runtimeMap[runtime]()
}

func (m *RuntimeManager) GetRuntimeMonitoring(runtime model.RuntimeType) RuntimeMonitoring {
	return m.runtimeMap[runtime]()
}

// can be used by registered runtimes to register additional sub-runtimes.
// E.g. containerd can register runc as well as urunc
func (m *RuntimeManager) registerRuntimeLink(name string, getter RuntimeGetter) {
	m.runtimeMap[model.RuntimeType(name)] = getter
}
