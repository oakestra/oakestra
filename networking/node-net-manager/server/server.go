package server

import (
	"NetManager/env"
	"NetManager/handlers"
	"NetManager/logger"
	"NetManager/mqtt"
	"NetManager/network"
	"NetManager/proxy"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/gorilla/mux"
)

type undeployRequest struct {
	Servicename    string `json:"serviceName"`
	Instancenumber int    `json:"instanceNumber"`
}

type registerRequest struct {
	ClientID       string `json:"client_id"`
	ClusterAddress string `json:"cluster_address"`
}

type DeployResponse struct {
	ServiceName string `json:"serviceName"`
	NsAddress   string `json:"nsAddress"`
}

type netConfiguration struct {
	NodePublicAddress string
	NodePublicPort    string
	ClusterUrl        string
	ClusterMqttPort   string
	Debug             bool
	MqttCert          string
	MqttKey           string
}

func HandleRequests(port int) {
	netRouter := mux.NewRouter().StrictSlash(true)
	netRouter.HandleFunc("/register", register).Methods("POST")

	//If default route, fetch default gateway address and use that.
	if Configuration.NodePublicAddress == "0.0.0.0" {
		defaultLink := network.GetOutboundIP()
		Configuration.NodePublicAddress = defaultLink.String()
	}

	handlers.RegisterAllManagers(&Env, &WorkerID, Configuration.NodePublicAddress, Configuration.NodePublicPort, netRouter)

	if port <= 0 {
		logger.InfoLogger().Println("Starting NetManager on unix socket /etc/netmanager/netmanager.sock")
		_ = os.Remove("/etc/netmanager/netmanager.sock")
		listener, err := net.Listen("unix", "/etc/netmanager/netmanager.sock")
		if err != nil {
			log.Fatal(err)
		}
		log.Fatal(http.Serve(listener, netRouter))
	} else {
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), netRouter))
	}
}

var (
	Env           env.Environment
	Proxy         proxy.GoProxyTunnel
	WorkerID      string
	Configuration netConfiguration
)

/*
Endpoint: /register
Usage: used to initialize the Network manager. The network manager must know his local subnetwork.
Method: POST
Request Json:

	{
		client_id:string # id of the worker node
	}

Response: 200 or Failure code
*/
func register(writer http.ResponseWriter, request *http.Request) {
	logger.InfoLogger().Println("Received registration request, registering the NetManager to the Cluster")

	reqBody, _ := io.ReadAll(request.Body)
	var requestStruct registerRequest
	err := json.Unmarshal(reqBody, &requestStruct)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
	}
	log.Println(requestStruct)

	// drop the request if the node is already initialized
	if WorkerID != "" {
		if WorkerID == requestStruct.ClientID {
			logger.InfoLogger().Printf("Node already initialized")
			writer.WriteHeader(http.StatusOK)
		} else {
			logger.InfoLogger().Printf("Attempting to re-initialize a node with a different worker ID")
			writer.WriteHeader(http.StatusBadRequest)
		}
		return
	}

	WorkerID = requestStruct.ClientID

	//Use default cluster address given by NodeEngine version >= v0.4.302
	if requestStruct.ClusterAddress != "" {
		Configuration.ClusterUrl = requestStruct.ClusterAddress
	}

	//log registration startup
	logger.InfoLogger().Printf(
		"STARTUP_CONFIG: Node=%s:%s | Cluster=%s:%s",
		Configuration.NodePublicAddress,
		Configuration.NodePublicPort,
		Configuration.ClusterUrl,
		Configuration.ClusterMqttPort,
	)

	// initialize mqtt connection to the broker
	mqtt.InitNetMqttClient(requestStruct.ClientID, Configuration.ClusterUrl, Configuration.ClusterMqttPort, Configuration.MqttCert, Configuration.MqttKey)

	// initialize the proxy tunnel
	Proxy = proxy.New()
	Proxy.Listen()

	// initialize the Env Manager
	Env = *env.NewEnvironmentClusterConfigured(Proxy.HostTUNDeviceName)

	Proxy.SetEnvironment(&Env)

	logger.InfoLogger().Printf("NetManager is now running 🟢")
	writer.WriteHeader(http.StatusOK)
}
