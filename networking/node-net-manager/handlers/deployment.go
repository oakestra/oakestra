package handlers

import (
	"NetManager/env"
	"NetManager/logger"
	"NetManager/mqtt"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
)

type ContainerDeployTask struct {
	Pid            int    `json:"pid"`
	ServiceName    string `json:"serviceName"`
	Instancenumber int    `json:"instanceNumber"`
	PortMappings   string `json:"portMappings"`
	Runtime        string
	PublicAddr     string
	PublicPort     string
	Env            *env.Environment
	Writer         *http.ResponseWriter
	Finish         chan TaskReady
}

type TaskReady struct {
	IP   net.IP
	IPv6 net.IP
	Err  error
}

type deployTaskQueue struct {
	newTask chan *ContainerDeployTask
}

type DeployTaskQueue interface {
	NewTask(request *ContainerDeployTask)
}

var (
	once      sync.Once
	taskQueue deployTaskQueue
)

func NewDeployTaskQueue() DeployTaskQueue {
	once.Do(func() {
		taskQueue = deployTaskQueue{
			newTask: make(chan *ContainerDeployTask, 50),
		}
		go taskQueue.taskExecutor()
	})
	return &taskQueue
}

func (t *deployTaskQueue) NewTask(request *ContainerDeployTask) {
	t.newTask <- request
}

func (t *deployTaskQueue) taskExecutor() {
	for {
		select {
		case task := <-t.newTask:
			// deploy the network stack in the container
			addr, addrv6, err := deploymentHandler(task)
			if err != nil {
				logger.ErrorLogger().Println("[ERROR]: ", err)
			}
			task.Finish <- TaskReady{
				IP:   addr,
				IPv6: addrv6,
				Err:  err,
			}
			// asynchronously update proxy tables
			updateInternalProxyDataStructures(task)
		}
	}
}

func deploymentHandler(requestStruct *ContainerDeployTask) (net.IP, net.IP, error) {
	// get app full name
	appCompleteName := strings.Split(requestStruct.ServiceName, ".")
	if len(appCompleteName) != 4 {
		return nil, nil, fmt.Errorf("invalid app name: %s", appCompleteName)
	}

	// attach network to the container
	netHandler := env.GetNetDeployment(requestStruct.Runtime)
	logger.DebugLogger().Printf("Got netHandler: %v", netHandler)
	addr, addrv6, err := netHandler.DeployNetwork(requestStruct.Pid, requestStruct.ServiceName, requestStruct.Instancenumber, requestStruct.PortMappings)
	if err != nil {
		logger.ErrorLogger().Println("[ERROR]:", err)
		return nil, nil, err
	}

	// notify to net-component
	err = mqtt.NotifyDeploymentStatus(
		requestStruct.ServiceName,
		"DEPLOYED",
		requestStruct.Instancenumber,
		addr.String(),
		addrv6.String(),
		requestStruct.PublicAddr,
		requestStruct.PublicPort,
	)
	if err != nil {
		logger.ErrorLogger().Println("[ERROR]:", err)
		return nil, nil, err
	}

	return addr, addrv6, nil
}

func updateInternalProxyDataStructures(requestStruct *ContainerDeployTask) {
	// Update internal table entry if an interest has not been set already.
	// Otherwise, do nothing, the net will autonomously update.
	if !mqtt.MqttIsInterestRegistered(requestStruct.ServiceName) {
		requestStruct.Env.RefreshServiceTable(requestStruct.ServiceName)
		mqtt.MqttRegisterInterest(requestStruct.ServiceName, requestStruct.Env, requestStruct.Instancenumber)
	}
}
