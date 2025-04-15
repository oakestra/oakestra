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

func GetRuntimeMonitoring(runtime model.RuntimeType) RuntimeMonitoring {
	return runtimeMap[runtime]()
}

// can be used by registered runtimes to register additional sub-runtimes.
// E.g. containerd can register runc as well as urunc
func registerRuntimeLink(name string, getter RuntimeGetter) {
	runtimeMap[model.RuntimeType(name)] = getter
}
