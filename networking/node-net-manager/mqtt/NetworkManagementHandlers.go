package mqtt

import (
	"encoding/json"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"log"
	"net"
	"time"
)

var subnetworkResponseChannel chan string

type mqttSubnetworkResponse struct {
	Address string `json:"address"`
}
type mqttSubnetworkRequest struct {
	METHOD string `json:"METHOD"`
}
type mqttDeployNotification struct {
	Appname string 	`json:"appname"`
	Status 	string 	`json:"status"`
	Instancenumber int `json:"instance_number"`
	Nsip 	string 	`json:"nsip"`
	Hostport string	`json:"host_port"`
	Hostip  string 	`json:"host_ip"`
}

func subnetworkAssignmentMqttHandler(client mqtt.Client, msg mqtt.Message) {
	responseStruct := mqttSubnetworkResponse{}
	err := json.Unmarshal(msg.Payload(), &responseStruct)
	if err != nil {
		log.Println("ERROR - Invalid subnetwork response")
		subnetworkResponseChannel <- ""
		return
	}
	subnetworkResponseChannel <- responseStruct.Address
}

/*Request a subnetwork to the cluster using the mqtt broker*/
func RequestSubnetworkMqttBlocking() (string, error) {
	subnetworkResponseChannel = make(chan string, 1)

	request := mqttSubnetworkRequest{METHOD: "GET"}
	jsonreq, _ := json.Marshal(request)
	go PublishToBroker("subnet", string(jsonreq))

	//waiting for maximum 10 seconds the mqtt handler to receive a response. Otherwise fail the subnetwork request.
	select {
	case result := <-subnetworkResponseChannel:
		if result != "" {
			return result, nil
		}
	case <-time.After(10 * time.Second):
		log.Printf("TIMEOUT - Table query without response, quitting goroutine")
	}

	return "", net.UnknownNetworkError("Invalid Subnetwork received")
}

func NotifyDeploymentStatus(appname string, status string, instance int, nsip string, hostip string, hostport string){
	request := mqttDeployNotification{
		Appname: appname,
		Status: status,
		Instancenumber: instance,
		Nsip: nsip,
		Hostip: hostip,
		Hostport: hostport,
	}
	jsonreq, _ := json.Marshal(request)
	PublishToBroker("service/deployed", string(jsonreq))
}
