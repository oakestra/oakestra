package virtualization

import (
	"go_node_engine/model"
	"time"
)

type RuntimeInterface interface {
	Deploy(service model.Service, statusChangeNotificationHandler func(service model.Service)) error
	Undeploy(sname string, instance int) error
}

type RuntimeMonitoring interface {
	ResourceMonitoring(every time.Duration, notifyHandler func(res []model.Resources))
}

type RuntimeType string

func GetRuntime(runtime string) RuntimeInterface {
	if runtime == model.CONTAINER_RUNTIME {
		return GetContainerdClient()
	}
	if runtime == model.UNIKERNEL_RUNTIME {
		return GetUnikernelRuntime()
	}
	return nil
}

func GetRuntimeMonitoring(runtime string) RuntimeMonitoring {
	if runtime == model.CONTAINER_RUNTIME {
		return GetContainerdClient()
	}
	return nil
}
