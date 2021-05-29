package main

import (
	"NetManager/env"
	"NetManager/proxy"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"io/ioutil"
	"log"
	"net"
	"net/http"
)

type deployRequest struct {
	serviceName string
	containerid string
}

type deployResponse struct {
	serviceName string
	nsAddress   string
}

type undeployRequest struct {
	servicename string
}

type registerRequest struct {
	subnetwork string
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

/*
Endpoint: /docker/undeploy
Usage: used to remove the network from a docker container
Method: POST
Request Json:
	{
		serviceName:string #name used to register the service in the first place
	}
Response: 200 OK or Failure code
*/
func dockerUndeploy(writer http.ResponseWriter, request *http.Request) {

}

/*
Endpoint: /docker/deploy
Usage: used to assign a network to a docker container
Method: POST
Request Json:
	{
		serviceName:string #name used to register the service in the first place
	}
Response Json:
	{
		serviceName:    string
		nsAddress:  	string # address assigned to this container
	}
*/
func dockerDeploy(writer http.ResponseWriter, request *http.Request) {
	reqBody, _ := ioutil.ReadAll(request.Body)
	var requestStruct deployRequest
	err := json.Unmarshal(reqBody, &requestStruct)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
	}

	//attach network to the container
	newAddr := generateAddr()
	Env.AttachDockerContainer(requestStruct.containerid, newAddr)

	response := deployResponse{
		serviceName: requestStruct.serviceName,
		nsAddress:   "",
	}

	err = json.NewEncoder(writer).Encode(response)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
	}
}

func generateAddr() net.IP {
	return net.ParseIP("")
}

/*
Endpoint: /register
Usage: used to initialize the Network manager. The network manager must know his local subnetwork.
Method: POST
Request Json:
	{
		subnetwork:string # IP address of the assigned subnetwork
	}
Response: 200 or Failure code
*/
func register(writer http.ResponseWriter, request *http.Request) {
	reqBody, _ := ioutil.ReadAll(request.Body)
	var requestStruct registerRequest
	err := json.Unmarshal(reqBody, &requestStruct)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
	}

	//initialize the node
	envconfig := env.Configuration{
		HostBridgeName:             "goProxyBridge",
		HostBridgeIP:               requestStruct.subnetwork,
		HostBridgeMask:             "/26",
		HostTunName:                "goProxyTun",
		ConnectedInternetInterface: "wlan0",
	}

	//assign the network

	writer.WriteHeader(http.StatusOK)
}

func main() {
	fmt.Println("Rest API v2.0 - Mux Routers")
	handleRequests()
}
