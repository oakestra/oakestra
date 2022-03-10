package virtualization

import (
	"go_node_engine/model"
)

type RuntimeInterface interface {
	Deploy(service model.Service) error
	Undeploy(sname string) error
}

type RuntimeType string

const (
	CONTAINER_RUNTIME = "docker"
	UNIKERNEL_RUNTIME = "unikernel"
)

func GetRuntime(runtime string) RuntimeInterface {
	if runtime == CONTAINER_RUNTIME {
		return GetContainerdClient()
	}
	return nil
}
