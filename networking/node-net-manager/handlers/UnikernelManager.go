package handlers

import (
	"NetManager/env"
	"NetManager/logger"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

type UnikernelManager struct {
	Env           *env.Environment
	WorkerID      *string
	Configuration netConfiguration
}

var unikernelManager *UnikernelManager

func init() {
	AvailableRuntimes[env.UNIKERNEL_RUNTIME] = GetUnikernelManager
	unikernelManager = &UnikernelManager{}
}

func GetUnikernelManager() ManagerInterface {
	return unikernelManager
}

func (m *UnikernelManager) Register(Env *env.Environment, WorkerID *string, NodePublicAddress string, NodePublicPort string, Router *mux.Router) {
	m.Env = Env
	m.WorkerID = WorkerID
	m.Configuration = netConfiguration{NodePublicAddress: NodePublicAddress, NodePublicPort: NodePublicPort}

	env.InitUnikernelDeployment(Env)

	Router.HandleFunc("/unikernel/deploy", m.CreateUnikernelNamesapce).Methods("POST")
	Router.HandleFunc("/unikernel/undeploy", m.DeleteUnikernelNamespace).Methods("POST")
}

/*
Endpoint: /unikernel/delpoy
Usage: used to create the network for the unikernel. Including a namespace, bridge and tap device
Method: POST
Request Json:

	{
		client_id:string # id of the worker node
		#TODO
	}

Response: 200 or Failure code
*/
func (m *UnikernelManager) CreateUnikernelNamesapce(writer http.ResponseWriter, request *http.Request) {
	log.Println("Received HTTP request - /unikernel/deploy")

	if *m.WorkerID == "" {
		log.Printf("[ERROR] Node not initialized")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	reqBody, _ := io.ReadAll(request.Body)
	log.Printf("ReqBody received :%s", reqBody)
	var requestStruct ContainerDeployTask
	err := json.Unmarshal(reqBody, &requestStruct)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
	}
	requestStruct.Runtime = env.UNIKERNEL_RUNTIME
	requestStruct.PublicAddr = m.Configuration.NodePublicAddress
	requestStruct.PublicPort = m.Configuration.NodePublicPort
	requestStruct.Env = m.Env
	requestStruct.Writer = &writer
	requestStruct.Finish = make(chan TaskReady, 0)
	logger.DebugLogger().Println(requestStruct)
	NewDeployTaskQueue().NewTask(&requestStruct)
	result := <-requestStruct.Finish
	if result.Err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	response := DeployResponse{
		ServiceName: requestStruct.ServiceName,
		NsAddress:   result.IP.String(),
		NsAddressv6: result.IPv6.String(),
	}

	logger.InfoLogger().Println("Response to /container/deploy: ", response)

	writer.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(writer).Encode(response)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
	}
}

/*
Endpoint: /unikernel/undeploy
Usage: used to remove the network from the unikernel env and delete the namespace associated with the unikernel.
Method: POST
Request Json:

	{
		serviceName:string #name used to register the service in a unikernel deploy request
	}

Response: 200 or Failure code
*/
func (m *UnikernelManager) DeleteUnikernelNamespace(writer http.ResponseWriter, request *http.Request) {
	log.Println("Received HTTP request - /unikernel/undeploy")

	if *m.WorkerID == "" {
		log.Printf("[ERROR] Node not initialized")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	reqBody, _ := io.ReadAll(request.Body)
	var requestStruct undeployRequest
	err := json.Unmarshal(reqBody, &requestStruct)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
	}

	log.Println(requestStruct)

	m.Env.DeleteUnikernelNamespace(requestStruct.Servicename, requestStruct.Instancenumber)

	writer.WriteHeader(http.StatusOK)
}
