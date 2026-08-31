package server

import (
	"NetManager/env"
	"NetManager/handlers"
	"NetManager/logger"
	"NetManager/model"
	"NetManager/mqtt"
	"NetManager/network"
	"NetManager/proxy"
	"context"
	"encoding/json"
	"errors"
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
	ClientID          string `json:"client_id"`
	ClusterAddress    string `json:"cluster_address"`
	NodePublicAddress string `json:"node_public_address"`
}

type DeployResponse struct {
	ServiceName string `json:"serviceName"`
	NsAddress   string `json:"nsAddress"`
}

func update() {
	for {
		time.Sleep(IP_UPDATE_TIMER)
		defaultLink := network.GetOutboundIP()
		if model.NetConfig.NodePublicAddress != defaultLink.String() {
			logger.InfoLogger().Printf("Updating NodePublicAddress from %s to %s", model.NetConfig.NodePublicAddress, defaultLink.String())
			// update service in the cluster
			//for each service instance in the worker, update the public address
			for _, si := range Env.GetTableEntriesOnNode() {
				err := mqtt.NotifyAddressChange(si.Appname, si.Instancenumber, defaultLink.String(), model.NetConfig.NodePublicPort)
				if err != nil {
					logger.ErrorLogger().Println("[ERROR]:", err)
				}
			}
			model.NetConfig.NodePublicAddress = defaultLink.String()
		}
	}
}

func createListener(port int) net.Listener {
	if port <= 0 {
		logger.InfoLogger().Println("Starting NetManager on unix socket /etc/netmanager/netmanager.sock")
		_ = os.Remove("/etc/netmanager/netmanager.sock")
		listener, err := net.Listen("unix", "/etc/netmanager/netmanager.sock")
		if err != nil {
			log.Fatalf("Could not create listener: %s", err)
		}
		return listener
	}
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("Could not create listener: %s", err)
	}
	return listener
}

func HandleRequests(port int) {
	// registration bootstrap server
	initRouter := mux.NewRouter().StrictSlash(true)
	initRouter.HandleFunc("/register", register).Methods("POST")

	initServer = &http.Server{Handler: initRouter}
	initListener := createListener(port)

	if err := initServer.Serve(initListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Registration server failed: %s", err)
	}

	// main server with all registered routes
	mainRouter := mux.NewRouter().StrictSlash(true)
	mainRouter.HandleFunc("/register", register).Methods("POST")
	handlers.RegisterAllManagers(&Env, &model.WorkerID, model.NetConfig.NodePublicAddress, model.NetConfig.NodePublicPort, mainRouter)

	mainListener := createListener(port)
	log.Fatal(http.Serve(mainListener, mainRouter))
}

var (
	Env        env.Environment
	Proxy      *proxy.GoProxyTunnel
	initServer *http.Server
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
		return
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

	// 0.0.0.0 is the default when netcfg.json is created
	if model.NetConfig.NodePublicAddress == "" || model.NetConfig.NodePublicAddress == "0.0.0.0" {
		if requestStruct.NodePublicAddress != "" {
			// when NodeEngine had an explicit public IP configured, use it
			model.NetConfig.NodePublicAddress = requestStruct.NodePublicAddress
		} else {
			// when NodeEngine had no IP configured, get the default outbound IP
			defaultLink := network.GetOutboundIP()
			model.NetConfig.NodePublicAddress = defaultLink.String()
			go update()
		}
	}

	model.WorkerID = requestStruct.ClientID

	//Use default cluster address given by NodeEngine version >= v0.4.302
	if requestStruct.ClusterAddress != "" {
		model.NetConfig.ClusterUrl = requestStruct.ClusterAddress
	}

	//log registration startup
	logger.InfoLogger().Printf(
		"STARTUP_CONFIG: Node=%s:%s | Cluster=%s:%s",
		model.NetConfig.NodePublicAddress,
		model.NetConfig.NodePublicPort,
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

	// Shut down stage 1 server so HandleRequests proceeds to stage 2
	if initServer != nil {
		go func() {
			_ = initServer.Shutdown(context.Background())
		}()
	}
}
