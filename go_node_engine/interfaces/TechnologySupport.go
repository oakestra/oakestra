package interfaces

import (
	"go_node_engine/model"
	"go_node_engine/virtualization"
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
		return virtualization.GetContainerdClient()
	}
	return nil
}
