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

type RuntimeType string

func GetRuntime(runtime model.RuntimeType) RuntimeInterface {
	if runtime == model.CONTAINER_RUNTIME {
		return GetContainerdClient()
	}
	if runtime == model.UNIKERNEL_RUNTIME {
		return GetUnikernelRuntime()
	}
	if runtime == model.WASM_RUNTIME {
		return GetWasmRuntime()
	}
	return nil
}

func GetRuntimeMonitoring(runtime model.RuntimeType) RuntimeMonitoring {
	if runtime == model.CONTAINER_RUNTIME {
		return GetContainerdClient()
	}
	if runtime == model.UNIKERNEL_RUNTIME {
		return GetUnikernelRuntime()
	}
	if runtime == model.WASM_RUNTIME {
		return GetWasmRuntime()
	}
	return nil
}
