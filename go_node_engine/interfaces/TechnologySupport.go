package interfaces

import (
	"go_node_engine/containers"
	"go_node_engine/model"
)

type RuntimeInterface interface {
	Deploy(service model.Service) error
	Undeploy(sname string) error
}

func GetRuntime(runtime string) RuntimeInterface {
	if runtime == "docker" {
		return containers.GetContainerdClient()
	}
	return nil
}
