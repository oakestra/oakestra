package server

import (
	"NetManager/env"
	"NetManager/handlers"
	"NetManager/logger"
	"NetManager/model"
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
	"time"

	"github.com/gorilla/mux"
)

const IP_UPDATE_TIMER = 2 * time.Minute

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

func update() {
	for {
		select {
		case <-time.After(IP_UPDATE_TIMER):
			func() {
				defaultLink := network.GetOutboundIP()
				if model.NetConfig.NodeAddress != defaultLink.String() {
					logger.InfoLogger().Printf("Updating NodeAddress from %s to %s", model.NetConfig.NodeAddress, defaultLink.String())
					defer func() { model.NetConfig.NodeAddress = defaultLink.String() }()
					// update service in the cluster
					//for each service instance in the worker, update the public address
					for _, si := range Env.GetTableEntriesOnNode() {
						err := mqtt.NotifyDeploymentStatus(si.Appname, "DEPLOYED", si.Instancenumber, si.Nsip.String(), si.Nsipv6.String(), defaultLink.String(), model.NetConfig.NodePort)
						if err != nil {
							logger.ErrorLogger().Println("[ERROR]:", err)
						}
					}
				}
			}()
		}
	}
}

func HandleRequests(port int) {
	netRouter := mux.NewRouter().StrictSlash(true)
	netRouter.HandleFunc("/register", register).Methods("POST")

	//If default route, fetch default gateway address and use that, update regularly
	if model.NetConfig.NodeAddress == "0.0.0.0" {
		defaultLink := network.GetOutboundIP()
		model.NetConfig.NodeAddress = defaultLink.String()
		go update()
	}

	handlers.RegisterAllManagers(&Env, &model.WorkerID, model.NetConfig.NodeAddress, model.NetConfig.NodePort, netRouter)

	if port <= 0 {
		logger.InfoLogger().Println("Starting NetManager on unix socket /etc/netmanager/netmanager.sock")
		_ = os.Remove("/etc/netmanager/netmanager.sock")
		listener, err := net.Listen("unix", "/etc/netmanager/netmanager.sock")
		if err != nil {
			log.Fatalf("Could not create listner: %s", err)
		}
		log.Fatal(http.Serve(listener, netRouter))
	} else {
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), netRouter))
	}
}

var (
	Env   env.Environment
	Proxy proxy.GoProxyTunnel
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
	if model.WorkerID != "" {
		if model.WorkerID == requestStruct.ClientID {
			logger.InfoLogger().Printf("Node already initialized")
			writer.WriteHeader(http.StatusOK)
		} else {
			logger.InfoLogger().Printf("Attempting to re-initialize a node with a different worker ID")
			writer.WriteHeader(http.StatusBadRequest)
		}
		return
	}

	model.WorkerID = requestStruct.ClientID

	//Use default cluster address given by NodeEngine version >= v0.4.302
	if requestStruct.ClusterAddress != "" {
		model.NetConfig.ClusterUrl = requestStruct.ClusterAddress
	}

	//log registration startup
	logger.InfoLogger().Printf(
		"STARTUP_CONFIG: Node=%s:%s | Cluster=%s:%s",
		model.NetConfig.NodeAddress,
		model.NetConfig.NodePort,
		model.NetConfig.ClusterUrl,
		model.NetConfig.ClusterMqttPort,
	)

	// initialize mqtt connection to the broker
	mqtt.InitNetMqttClient(requestStruct.ClientID, model.NetConfig.ClusterUrl, model.NetConfig.ClusterMqttPort, model.NetConfig.MqttCert, model.NetConfig.MqttKey)

	// initialize the proxy tunnel
	Proxy = proxy.New()
	Proxy.Listen()

	// initialize the Env Manager
	Env = *env.NewEnvironmentClusterConfigured(Proxy.HostTUNDeviceName)

	Proxy.SetEnvironment(&Env)

	logger.InfoLogger().Printf("NetManager is now running 🟢")
	writer.WriteHeader(http.StatusOK)
}
