package virtualization

import (
	"go_node_engine/model"
	"time"
)

var runtimeMap = map[model.RuntimeType]Runtime{}

type RuntimeInterface interface {
	Deploy(service model.Service, statusChangeNotificationHandler func(service model.Service)) error
	Undeploy(sname string, instance int) error
}

type RuntimeMonitoring interface {
	ResourceMonitoring(every time.Duration, notifyHandler func(res []model.Resources))
}

type Runtime interface {
	RuntimeInterface
	RuntimeMonitoring
}

type RuntimeType string

func GetRuntime(runtime model.RuntimeType) RuntimeInterface {
	return runtimeMap[runtime]
}

func GetRuntimeMonitoring(runtime model.RuntimeType) RuntimeMonitoring {
	return runtimeMap[runtime]
}

func RegisterRuntime(runtime model.RuntimeType, runtimeInterface Runtime) {
	runtimeMap[runtime] = runtimeInterface
}
