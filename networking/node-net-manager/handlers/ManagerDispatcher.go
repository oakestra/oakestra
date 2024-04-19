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
	Register(Env *env.Environment, WorkerID *string, NodePublicAddress string, NodePublicPort string, Router *mux.Router)
}

func GetNetManager(handler string) ManagerInterface {
	if getfunc, ok := AvailableRuntimes[handler]; ok {
		return getfunc()
	}
	return nil
}

func RegisterAllManagers(Env *env.Environment, WorkerID *string, NodePublicAddress string, NodePublicPort string, Router *mux.Router) {
	for _, getfunc := range AvailableRuntimes {
		getfunc().Register(Env, WorkerID, NodePublicAddress, NodePublicPort, Router)
	}
}
