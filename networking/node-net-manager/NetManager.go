package main

import (
	"NetManager/env"
	"NetManager/handlers"
	"NetManager/mqtt"
	"NetManager/playground"
	"NetManager/proxy"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/tkanos/gonfig"
	"io/ioutil"
	"log"
	"net/http"
)

type undeployRequest struct {
	Servicename string `json:"serviceName"`
}

type registerRequest struct {
	ClientID string `json:"client_id"`
}

type netConfiguration struct {
	NodePublicAddress string
	NodePublicPort    string
	ClusterUrl        string
	ClusterMqttPort   string
}

func handleRequests(port int) {
	netRouter := mux.NewRouter().StrictSlash(true)
	netRouter.HandleFunc("/register", register).Methods("POST")
	netRouter.HandleFunc("/docker/deploy", dockerDeploy).Methods("POST")
	netRouter.HandleFunc("/container/deploy", containerDeploy).Methods("POST")
	netRouter.HandleFunc("/docker/undeploy", containerUndeploy).Methods("POST")
	netRouter.HandleFunc("/container/undeploy", containerUndeploy).Methods("POST")
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), netRouter))
}

var Env *env.Environment
var Proxy proxy.GoProxyTunnel
var WorkerID string
var Configuration netConfiguration

/*
Endpoint: /docker/undeploy
Usage: used to remove the network from a docker container. This method can be used only after the registration
Method: POST
Request Json:
	{
		serviceName:string #name used to register the service in the first place
	}
Response: 200 OK or Failure code
*/
func containerUndeploy(writer http.ResponseWriter, request *http.Request) {
	log.Println("Received HTTP request - /docker/undeploy ")

	if WorkerID == "" {
		log.Printf("[ERROR] Node not initialized")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	reqBody, _ := ioutil.ReadAll(request.Body)
	var requestStruct undeployRequest
	err := json.Unmarshal(reqBody, &requestStruct)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
	}

	log.Println(requestStruct)

	Env.DetachContainer(requestStruct.Servicename)

	writer.WriteHeader(http.StatusOK)
}

/* DEPRECATED
Endpoint: /docker/deploy
Usage: used to assign a network to a docker container. This method can be used only after the registration
Method: POST
Request Json:
	{
		containerId:string #name of the container or containerid
		appName:string
		instanceNumber:int
	}
Response Json:
	{
		serviceName:    string
		nsAddress:  	string # address assigned to this container
	}
*/
func dockerDeploy(writer http.ResponseWriter, request *http.Request) {
	log.Println("Received HTTP request - /docker/deploy ")
	writer.WriteHeader(299)
	_, _ = writer.Write([]byte("DEPRECATED API"))
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
func containerDeploy(writer http.ResponseWriter, request *http.Request) {
	log.Println("Received HTTP request - /container/deploy ")

	if WorkerID == "" {
		log.Printf("[ERROR] Node not initialized")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	reqBody, _ := ioutil.ReadAll(request.Body)
	log.Println("ReqBody received :", reqBody)
	var requestStruct handlers.ContainerDeployRequest
	err := json.Unmarshal(reqBody, &requestStruct)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
	}
	requestStruct.PublicAddr = Configuration.NodePublicAddress
	requestStruct.PublicPort = Configuration.NodePublicPort
	requestStruct.Env = Env
	requestStruct.Writer = &writer
	requestStruct.Finish = make(chan bool, 0)
	log.Println(requestStruct)

	handlers.NewDeployTaskQueue().NewTask(&requestStruct)
	<-requestStruct.Finish
}

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
	log.Println("Received HTTP request - /register ")

	reqBody, _ := ioutil.ReadAll(request.Body)
	var requestStruct registerRequest
	err := json.Unmarshal(reqBody, &requestStruct)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
	}
	log.Println(requestStruct)

	//drop the request if the node is already initialized
	if WorkerID != "" {
		if WorkerID == requestStruct.ClientID {
			log.Printf("Node already initialized")
			writer.WriteHeader(http.StatusOK)
		} else {
			log.Printf("Attempting to re-initialize a node with a different worker ID")
			writer.WriteHeader(http.StatusBadRequest)
		}
		return
	}

	WorkerID = requestStruct.ClientID

	//initialize mqtt connection to the broker
	mqtt.InitMqtt(requestStruct.ClientID, Configuration.ClusterUrl, Configuration.ClusterMqttPort)

	//initialize the proxy tunnel
	Proxy = proxy.New()
	Proxy.Listen()

	//initialize the Env Manager
	Env = env.NewEnvironmentClusterConfigured(Proxy.HostTUNDeviceName)
	Proxy.SetEnvironment(Env)

	writer.WriteHeader(http.StatusOK)
}

func main() {

	cfgFile := flag.String("cfg", "/etc/netmanager/netcfg.json", "Set a cluster IP")
	localPort := flag.Int("p", 10010, "Default local port of the NetManager")
	p2pMode := flag.Bool("p2p", false, "Start the engine in p2p mode (playground2playground), requires the address of a peer node. Useful for debugging.")
	flag.Parse()

	err := gonfig.GetConf(*cfgFile, &Configuration)
	if err != nil {
		log.Fatal(err)
	}

	log.Print(Configuration)

	env.IptableFlushAll()

	if *p2pMode {
		defer playground.APP.Stop()
		playground.CliLoop(Configuration.NodePublicAddress, Configuration.NodePublicPort)
	}

	log.Println("NetManager started. Waiting for registration.")
	handleRequests(*localPort)
}
