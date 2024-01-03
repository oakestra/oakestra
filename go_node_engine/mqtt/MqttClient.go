package mqtt

import (
	"encoding/json"
	"fmt"
	"github.com/eclipse/paho.mqtt.golang"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/virtualization"
	"strings"
	"time"
)

var TOPICS = make(map[string]mqtt.MessageHandler)

var clientID = ""
var mainMqttClient mqtt.Client
var BrokerUrl = ""
var BrokerPort = ""

var messagePubHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	logger.InfoLogger().Printf("DEBUG - Received message: %s from topic: %s\n", msg.Payload(), msg.Topic())
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	logger.InfoLogger().Println("Connected to the MQTT broker")

	topicsQosMap := make(map[string]byte)
	for key, _ := range TOPICS {
		topicsQosMap[key] = 1
	}

	//subscribe to all the topics
	tqtoken := client.SubscribeMultiple(topicsQosMap, subscribeHandlerDispatcher)
	tqtoken.Wait()
	logger.InfoLogger().Printf("Subscribed to topics \n")

}

var subscribeHandlerDispatcher = func(client mqtt.Client, msg mqtt.Message) {
	for key, handler := range TOPICS {
		if strings.Contains(msg.Topic(), key) {
			handler(client, msg)
		}
	}
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	logger.InfoLogger().Printf("Connect lost: %v", err)
}

func InitMqtt(clientid string, brokerurl string, brokerport string) {

	if clientID != "" {
		logger.InfoLogger().Printf("Mqtt already initialized no need for any further initialization")
		return
	}

	BrokerPort = brokerport
	BrokerUrl = brokerurl

	//platform's assigned client ID
	clientID = clientid

	TOPICS[fmt.Sprintf("nodes/%s/control/deploy", clientID)] = deployHandler
	TOPICS[fmt.Sprintf("nodes/%s/control/delete", clientID)] = deleteHandler

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%s", BrokerUrl, BrokerPort))
	opts.SetClientID(clientid + "-ne")
	opts.SetUsername("")
	opts.SetPassword("")
	opts.SetDefaultPublishHandler(messagePubHandler)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler

	go runMqttClient(opts)
}

func runMqttClient(opts *mqtt.ClientOptions) {
	mainMqttClient = mqtt.NewClient(opts)
	if token := mainMqttClient.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
}

func publishToBroker(topic string, payload string) {
	logger.InfoLogger().Printf("MQTT - publish to - %s - the payload - %s", topic, payload)
	token := mainMqttClient.Publish(fmt.Sprintf("nodes/%s/%s", clientID, topic), 1, false, payload)
	if token.WaitTimeout(time.Second*5) && token.Error() != nil {
		logger.ErrorLogger().Printf("ERROR: MQTT PUBLISH: %s", token.Error())
	}
}

func deployHandler(client mqtt.Client, msg mqtt.Message) {
	logger.InfoLogger().Printf("Received deployment request with payload: %s", string(msg.Payload()))
	service := model.Service{}
	err := json.Unmarshal(msg.Payload(), &service)
	logger.InfoLogger().Printf("%v", service)
	if err != nil {
		logger.ErrorLogger().Printf("ERROR: unable to unmarshal cluster orch request: %v", err)
		return
	}
	//handle deployment in background
	go func() {
		runtime := virtualization.GetRuntime(model.RuntimeType(service.Runtime))
		err = runtime.Deploy(service, ReportServiceStatus)
		service.Status = model.SERVICE_ACTIVE
		if err != nil {
			logger.ErrorLogger().Printf("ERROR during app deployment: %v", err)
			service.StatusDetail = err.Error()
			service.Status = model.SERVICE_FAILED
		}
		ReportServiceStatus(service)
	}()
}
func deleteHandler(client mqtt.Client, msg mqtt.Message) {
	logger.InfoLogger().Printf("Received undeployment request with payload: %s", string(msg.Payload()))
	service := model.Service{}
	err := json.Unmarshal(msg.Payload(), &service)
	if err != nil {
		logger.ErrorLogger().Printf("ERROR: unable to unmarshal cluster orch request: %v", err)
		return
	}
	runtime := virtualization.GetRuntime(model.RuntimeType(service.Runtime))
	err = runtime.Undeploy(service.Sname, service.Instance)
	if err != nil {
		logger.ErrorLogger().Printf("Unable to undeploy application: %s", err.Error())
		return
	}
	service.Status = model.SERVICE_UNDEPLOYED
	ReportServiceStatus(service)
}

func ReportServiceStatus(service model.Service) {
	type ServiceStatus struct {
		Sname    string `json:"sname"`
		Status   string `json:"status"`
		Detail   string `json:"status_detail"`
		Instance int    `json:"instance"`
		Publicip string `json:"publicip"`
	}
	reportStatusStruct := ServiceStatus{
		Sname:    service.Sname,
		Status:   service.Status,
		Detail:   service.StatusDetail,
		Instance: service.Instance,
		Publicip: model.GetNodeInfo().Ip,
	}
	jsonmsg, err := json.Marshal(reportStatusStruct)
	if err != nil {
		logger.ErrorLogger().Printf("ERROR: unable to report service status: %v", err)
	}
	publishToBroker("job", string(jsonmsg))
}

func ReportServiceResources(services []model.Resources) {
	type ServiceResources struct {
		Services []model.Resources `json:"services"`
	}
	reportStatusStruct := ServiceResources{
		Services: services,
	}
	jsonmsg, err := json.Marshal(reportStatusStruct)
	if err != nil {
		logger.ErrorLogger().Printf("ERROR: unable to report services resources: %v", err)
	}
	publishToBroker("jobs/resources", string(jsonmsg))
}

func ReportNodeInformation(node model.Node) {
	data, err := json.Marshal(node)
	if err != nil {
		logger.ErrorLogger().Printf("ERROR: error gathering node info")
	}
	publishToBroker("information", string(data))
}
