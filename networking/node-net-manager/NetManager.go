package main

import (
	"NetManager/env"
	"NetManager/mqtt"
	"NetManager/proxy"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/tkanos/gonfig"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

type deployRequest struct {
	ContainerId    string `json:"containerId"`
	AppFullName    string `json:"appName"`
	Instancenumber int    `json:"instanceNumber"`
}

type sip struct {
	Type    string `json:"IpType"` //RR, Closest or InstanceNumber
	Address string `json:"Address"`
}

type deployResponse struct {
	ServiceName string `json:"serviceName"`
	NsAddress   string `json:"nsAddress"`
}

type undeployRequest struct {
	Servicename string `json:"servicename"`
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

func handleRequests() {
	netRouter := mux.NewRouter().StrictSlash(true)
	netRouter.HandleFunc("/register", register).Methods("POST")
	netRouter.HandleFunc("/docker/deploy", dockerDeploy).Methods("POST")
	netRouter.HandleFunc("/docker/undeploy", dockerUndeploy).Methods("POST")
	log.Fatal(http.ListenAndServe(":10010", netRouter))
}

var Env env.Environment
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
func dockerUndeploy(writer http.ResponseWriter, request *http.Request) {
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

	Env.DetachDockerContainer(requestStruct.Servicename)

	writer.WriteHeader(http.StatusOK)
}

/*
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

	if WorkerID == "" {
		log.Printf("[ERROR] Node not initialized")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	reqBody, _ := ioutil.ReadAll(request.Body)
	log.Println("ReqBody received :", reqBody)
	var requestStruct deployRequest
	err := json.Unmarshal(reqBody, &requestStruct)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
	}

	log.Println(requestStruct)

	//get app full name
	appCompleteName := strings.Split(requestStruct.AppFullName, ".")
	if len(appCompleteName) != 4 {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	//attach network to the container
	addr, err := Env.AttachDockerContainer(requestStruct.ContainerId)
	if err != nil {
		log.Println("[ERROR]:", err)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	//notify net-component
	mqtt.NotifyDeploymentStatus(
		requestStruct.AppFullName,
		"DEPLOYED",
		requestStruct.Instancenumber,
		addr.String(),
		Configuration.NodePublicAddress,
		Configuration.NodePublicPort,
	)

	//update internal table entry
	Env.RefreshServiceTable(requestStruct.AppFullName)

	//answer the caller
	response := deployResponse{
		ServiceName: requestStruct.AppFullName,
		NsAddress:   addr.String(),
	}

	log.Println("Response to /docker/deploy: ", response)

	writer.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(writer).Encode(response)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
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
	Proxy.SetEnvironment(&Env)

	//create debug netns
	_, err = Env.CreateNetworkNamespaceNewIp("debugAppNs")
	if err != nil {
		fmt.Println(err)
	}

	writer.WriteHeader(http.StatusOK)
}

func main() {
	cfgFile := flag.String("cfg", "/etc/netmanager/netcfg.json", "Set a cluster IP")
	flag.Parse()

	err := gonfig.GetConf(*cfgFile, &Configuration)
	if err != nil {
		log.Fatal(err)
	}

	log.Print(Configuration)

	log.Println("NetManager started. Waiting for registration.")
	handleRequests()
}
