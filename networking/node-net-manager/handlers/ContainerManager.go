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

type ContainerManager struct {
	Env           *env.Environment
	WorkerID      *string
	Configuration netConfiguration
}

var containerManager *ContainerManager

func init() {
	AvailableRuntimes[env.CONTAINER_RUNTIME] = GetContainerManager
	containerManager = &ContainerManager{}
}

func GetContainerManager() ManagerInterface {
	return containerManager
}

func (m *ContainerManager) Register(Env *env.Environment, WorkerID *string, NodePublicAddress string, NodePublicPort string, Router *mux.Router) {
	m.Env = Env
	m.WorkerID = WorkerID
	m.Configuration = netConfiguration{NodePublicAddress: NodePublicAddress, NodePublicPort: NodePublicPort}

	env.InitContainerDeployment(Env)
	Router.HandleFunc("/container/deploy", m.containerDeploy).Methods("POST")
	Router.HandleFunc("/container/undeploy", m.containerUndeploy).Methods("POST")
	Router.HandleFunc("/docker/undeploy", m.containerUndeploy).Methods("POST")
}

/*
Endpoint: /container/deploy
Usage: used to assign a network to a generic container. This method can be used only after the registration
Method: POST
Request Json:

	{
		pid:string #pid of container's task
		appName:string
		instanceNumber:int
		portMapppings: map[int]int (host port, container port)
	}

Response Json:

	{
		serviceName:    string
		nsAddress:  	string # address assigned to this container
	}
*/
func (m *ContainerManager) containerDeploy(writer http.ResponseWriter, request *http.Request) {
	log.Println("Received HTTP request - /container/deploy ")

	if *m.WorkerID == "" {
		log.Printf("[ERROR] Node not initialized")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	reqBody, _ := io.ReadAll(request.Body)
	log.Println("ReqBody received :", reqBody)
	var deployTask ContainerDeployTask
	err := json.Unmarshal(reqBody, &deployTask)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
	}
	deployTask.Runtime = env.CONTAINER_RUNTIME
	deployTask.PublicAddr = m.Configuration.NodePublicAddress
	deployTask.PublicPort = m.Configuration.NodePublicPort
	deployTask.Env = m.Env
	deployTask.Writer = &writer
	deployTask.Finish = make(chan TaskReady)

	logger.DebugLogger().Println(deployTask)
	NewDeployTaskQueue().NewTask(&deployTask)

	result := <-deployTask.Finish
	if result.Err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	//if deploy succesfull -> answer the caller
	response := DeployResponse{
		ServiceName: deployTask.ServiceName,
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
Endpoint: /docker/undeploy
Usage: used to remove the network from a docker container. This method can be used only after the registration
Method: POST
Request Json:

	{
		serviceName:string #name used to register the service in the first place
		instance:int
	}

Response: 200 OK or Failure code
*/
func (m *ContainerManager) containerUndeploy(writer http.ResponseWriter, request *http.Request) {
	log.Println("Received HTTP request - /container/undeploy ")

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

	m.Env.DetachContainer(requestStruct.Servicename, requestStruct.Instancenumber)

	writer.WriteHeader(http.StatusOK)
}
