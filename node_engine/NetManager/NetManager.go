package main

import (
	"NetManager/env"
	"NetManager/proxy"
	"encoding/json"
	"github.com/gorilla/mux"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
)

type deployRequest struct {
	ContainerId    string `json:"containerId"`
	AppFullName    string `json:"appName"`
	Instancenumber int    `json:"instanceNumber"`
	Nodeip         string `json:"nodeIp"`
	Nodeport       int    `json:"nodePort"`
	ServiceIP      []sip  `json:"serviceIp"`
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
	Subnetwork string `json:"subnetwork"`
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
var InitializationCompleted = false

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

	if !InitializationCompleted {
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
		nodeIp:string
		nodePort:int
		serviceIp:[{
					IpType:string //RR, Closest or InstanceNumber
					Address:string
					}]
	}
Response Json:
	{
		serviceName:    string
		nsAddress:  	string # address assigned to this container
	}
*/
func dockerDeploy(writer http.ResponseWriter, request *http.Request) {
	log.Println("Received HTTP request - /docker/deploy ")

	if !InitializationCompleted {
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

	//update internal table entry
	siplist := make([]env.ServiceIP, 0)
	for _, ip := range requestStruct.ServiceIP {
		siplist = append(siplist, env.ToServiceIP(ip.Type, ip.Address))
	}
	entry := env.TableEntry{
		Appname:          appCompleteName[0],
		Appns:            appCompleteName[1],
		Servicename:      appCompleteName[2],
		Servicenamespace: appCompleteName[3],
		Instancenumber:   requestStruct.Instancenumber,
		Cluster:          0,
		Nodeip:           net.ParseIP(requestStruct.Nodeip),
		Nodeport:         requestStruct.Nodeport,
		Nsip:             addr,
		ServiceIP:        siplist,
	}
	Env.AddTableQueryEntry(entry)

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
		subnetwork:string # IP address of the assigned subnetwork
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

	//initialize the proxy tunnel
	Proxy = proxy.New()

	//initialize the Env Manager
	Env = env.NewDefault(Proxy.HostTUNDeviceName, requestStruct.Subnetwork)

	//set initialization flag
	InitializationCompleted = true

	//create debug netns
	_, err = Env.CreateNetworkNamespaceNewIp("debugAppNs")
	if err != nil {
		print(err)
	}

	writer.WriteHeader(http.StatusOK)
}

func main() {
	log.Println("NetManager started. Waiting for registration.")
	handleRequests()
}
