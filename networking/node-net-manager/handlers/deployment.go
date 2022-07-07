package handlers

import (
	"NetManager/env"
	"NetManager/mqtt"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
)

type ContainerDeployRequest struct {
	Pid            int    `json:"pid"`
	ServiceName    string `json:"serviceName"`
	Instancenumber int    `json:"instanceNumber"`
	PortMappings   string `json:"portMappings"`
	PublicAddr     string
	PublicPort     string
	Env            *env.Environment
	Writer         *http.ResponseWriter
	Finish         chan bool
}

type DeployResponse struct {
	ServiceName string `json:"serviceName"`
	NsAddress   string `json:"nsAddress"`
}

type deployTaskQueue struct {
	newTask chan *ContainerDeployRequest
}

type DeployTaskQueue interface {
	NewTask(request *ContainerDeployRequest)
}

var once sync.Once
var taskQueue deployTaskQueue

func NewDeployTaskQueue() DeployTaskQueue {

	once.Do(func() {
		taskQueue = deployTaskQueue{
			newTask: make(chan *ContainerDeployRequest, 50),
		}
		go taskQueue.taskExecutor()
	})
	return &taskQueue
}

func (t *deployTaskQueue) NewTask(request *ContainerDeployRequest) {
	t.newTask <- request
}

func (t *deployTaskQueue) taskExecutor() {
	for true {
		select {
		case task := <-t.newTask:
			deploymentHandler(task)
			task.Finish <- true
		}
	}
}

func deploymentHandler(requestStruct *ContainerDeployRequest) {
	writer := *requestStruct.Writer

	//get app full name
	appCompleteName := strings.Split(requestStruct.ServiceName, ".")
	if len(appCompleteName) != 4 {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	//attach network to the container
	addr, err := requestStruct.Env.AttachNetworkToContainer(requestStruct.Pid, requestStruct.ServiceName, requestStruct.Instancenumber, requestStruct.PortMappings)
	if err != nil {
		log.Println("[ERROR]:", err)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	//notify net-component
	mqtt.NotifyDeploymentStatus(
		requestStruct.ServiceName,
		"DEPLOYED",
		requestStruct.Instancenumber,
		addr.String(),
		requestStruct.PublicAddr,
		requestStruct.PublicPort,
	)

	//update internal table entry
	requestStruct.Env.RefreshServiceTable(requestStruct.ServiceName)

	mqtt.MqttRegisterInterest(requestStruct.ServiceName, requestStruct.Env)

	//answer the caller
	response := DeployResponse{
		ServiceName: requestStruct.ServiceName,
		NsAddress:   addr.String(),
	}

	log.Println("Response to /container/deploy: ", response)

	writer.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(writer).Encode(response)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
	}
}
