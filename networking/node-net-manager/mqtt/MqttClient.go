package mqtt

import (
	"fmt"
	"github.com/eclipse/paho.mqtt.golang"
	"log"
	"os"
	"strconv"
	"strings"
)

var DEFAULT_BROKER_PORT = 1883
var DEFAULT_BROKER_URL = "localhost"
var DEFAULT_MQTT_USERNAME = ""
var DEFAULT_MQTT_PW = ""
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
		topicsQosMap[key]=1
	}

	//subscribe to all the topics
	tqtoken := client.SubscribeMultiple(topicsQosMap, subscribeHandlerDispatcher)
	tqtoken.Wait()
	log.Printf("Subscribed to topics \n")

	//subscribe to network management topics (interests messages and related) TODO
}

var subscribeHandlerDispatcher = func(client mqtt.Client, msg mqtt.Message) {
	for key, handler := range TOPICS {
		if strings.Contains(msg.Topic(),key) {
			handler(client,msg)
		}
	}
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	log.Printf("Connect lost: %v", err)
}

func InitMqtt(clientid string) {
	//platform's assigned client ID
	clientID = clientid
	tableQueryRequestCache = GetTableQueryRequestCacheInstance()

	brokerurl := os.Getenv("MQTT_BROKER_URL")
	if brokerurl == "" {
		log.Printf("INFO - mqtt broker url not found, switching to default %s", DEFAULT_BROKER_URL)
	}

	brokerport, porterr := strconv.Atoi(os.Getenv("MQTT_BROKER_PORT"))
	if porterr != nil {
		log.Printf("INFO - mqtt broker port not found, switching to default %d", DEFAULT_BROKER_PORT)
		brokerport = 1883
	}

	username := os.Getenv("MQTT_USERNAME")
	if username == "" {
		log.Printf("INFO - mqtt broker username not found, switching to default %d", DEFAULT_MQTT_USERNAME)
		username = DEFAULT_MQTT_USERNAME
	}

	password := os.Getenv("MQTT_PASSWORD")
	if password == "" {
		log.Printf("INFO - mqtt broker password not found, switching to default")
		password = DEFAULT_MQTT_PW
	}

	TOPICS[fmt.Sprintf("nodes/%s/net/tablequery/result", clientID)]=
		tableQueryRequestCache.TablequeryResultMqttHandler
	TOPICS[fmt.Sprintf("nodes/%s/net/subnetwork/result", clientID)]=
		subnetworkAssignmentMqttHandler

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", brokerurl, brokerport))
	opts.SetClientID(clientid)
	opts.SetUsername(username)
	opts.SetPassword(password)
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
	client.Publish(fmt.Sprintf("nodes/%s/net/%s", clientID, topic), 1, false, payload)
}
