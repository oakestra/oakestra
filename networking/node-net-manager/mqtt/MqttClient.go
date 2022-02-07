package mqtt

import (
	"fmt"
	"github.com/eclipse/paho.mqtt.golang"
	"log"
	"strings"
	"time"
)

var TOPICS = make(map[string]mqtt.MessageHandler)

var clientID = ""
var client mqtt.Client

var tableQueryRequestCache *TableQueryRequestCache

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

	//subscribe to network management topics (interests messages and related) TODO
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

	//platform's assigned client ID
	clientID = clientid
	tableQueryRequestCache = GetTableQueryRequestCacheInstance()

	TOPICS[fmt.Sprintf("nodes/%s/net/tablequery/result", clientID)] =
		tableQueryRequestCache.TablequeryResultMqttHandler
	TOPICS[fmt.Sprintf("nodes/%s/net/subnetwork/result", clientID)] =
		subnetworkAssignmentMqttHandler

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%s", brokerurl, brokerport))
	opts.SetClientID(clientid)
	opts.SetUsername("")
	opts.SetPassword("")
	opts.SetDefaultPublishHandler(messagePubHandler)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler

	go runMqttClient(opts)
}

func runMqttClient(opts *mqtt.ClientOptions) {
	client = mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
}

func PublishToBroker(topic string, payload string) {
	log.Printf("MQTT - publish to - %s - the payload - %s", topic, payload)
	token := client.Publish(fmt.Sprintf("nodes/%s/net/%s", clientID, topic), 1, false, payload)
	if token.WaitTimeout(time.Second*5) && token.Error() != nil {
		log.Printf("ERROR: MQTT PUBLISH: %s", token.Error())
	}
}
