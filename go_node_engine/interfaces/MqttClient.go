package interfaces

import (
	"fmt"
	"github.com/eclipse/paho.mqtt.golang"
	"log"
	"strings"
	"time"
)

var TOPICS = make(map[string]mqtt.MessageHandler)

var clientID = ""
var mainMqttClient mqtt.Client
var BrokerUrl = ""
var BrokerPort = ""

var messagePubHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	log.Printf("DEBUG - Received message: %s from topic: %s\n", msg.Payload(), msg.Topic())
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	log.Println("Connected to the MQTT broker")

	topicsQosMap := make(map[string]byte)
	for key, _ := range TOPICS {
		topicsQosMap[key] = 1
	}

	//subscribe to all the topics
	tqtoken := client.SubscribeMultiple(topicsQosMap, subscribeHandlerDispatcher)
	tqtoken.Wait()
	log.Printf("Subscribed to topics \n")

}

var subscribeHandlerDispatcher = func(client mqtt.Client, msg mqtt.Message) {
	for key, handler := range TOPICS {
		if strings.Contains(msg.Topic(), key) {
			handler(client, msg)
		}
	}
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	log.Printf("Connect lost: %v", err)
}

func InitMqtt(clientid string, brokerurl string, brokerport string) {

	if clientID != "" {
		log.Printf("Mqtt already initialized no need for any further initialization")
		return
	}

	BrokerPort = brokerport
	BrokerUrl = brokerurl

	//platform's assigned client ID
	clientID = clientid

	TOPICS[fmt.Sprintf("nodes/%s/control/deploy$", clientID)] = deployHandler
	TOPICS[fmt.Sprintf("nodes/%s/control/delete$", clientID)] = deleteHandler

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%s", BrokerUrl, BrokerPort))
	opts.SetClientID(clientid)
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

func PublishToBroker(topic string, payload string) {
	log.Printf("MQTT - publish to - %s - the payload - %s", topic, payload)
	token := mainMqttClient.Publish(fmt.Sprintf("nodes/%s/%s", clientID, topic), 1, false, payload)
	if token.WaitTimeout(time.Second*5) && token.Error() != nil {
		log.Printf("ERROR: MQTT PUBLISH: %s", token.Error())
	}
}

func deployHandler(client mqtt.Client, msg mqtt.Message) {
	//TODO
}
func deleteHandler(client mqtt.Client, msg mqtt.Message) {
	//TODO
}
