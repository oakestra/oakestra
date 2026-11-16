package handlers

import (
	"NetManager/env"

	"github.com/gorilla/mux"
)

type netConfiguration struct {
	NodePublicAddress string
	NodePublicPort    string
}

type undeployRequest struct {
	Servicename    string `json:"serviceName"`
	Instancenumber int    `json:"instanceNumber"`
}

type DeployResponse struct {
	ServiceName string `json:"serviceName"`
	NsAddress   string `json:"nsAddress"`
	NsAddressv6 string `json:"nsAddressv6"`
}

var AvailableRuntimes = make(map[string]func() ManagerInterface)

type ManagerInterface interface {
	Register(WorkerID *string, NodePublicAddress string, NodePublicPort string, Router *mux.Router)
	SetEnvironment(Env *env.Environment)
}

func GetNetManager(handler string) ManagerInterface {
	if getfunc, ok := AvailableRuntimes[handler]; ok {
		return getfunc()
	}
	return nil
}

// RegisterAllManagers wires up routes at startup, before the Environment exists.
func RegisterAllManagers(WorkerID *string, NodePublicAddress string, NodePublicPort string, Router *mux.Router) {
	for _, getfunc := range AvailableRuntimes {
		getfunc().Register(WorkerID, NodePublicAddress, NodePublicPort, Router)
	}
}

// SetEnvironmentForAllManagers binds the Environment once /register has created it.
func SetEnvironmentForAllManagers(Env *env.Environment) {
	for _, getfunc := range AvailableRuntimes {
		getfunc().SetEnvironment(Env)
	}
}
