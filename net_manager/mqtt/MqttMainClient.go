package mqtt

import (
	"NetManager/logger"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var initMqttClient sync.Once

type NetMqttClient struct {
	topics                 map[string]mqtt.MessageHandler
	clientID               string
	mainMqttClient         mqtt.Client
	mqttClientMutex        *sync.RWMutex
	brokerUrl              string
	brokerPort             string
	mqttCert               string
	mqttKey                string
	mqttCa                 string
	mqttWriteMutex         *sync.Mutex
	mqttTopicsMutex        *sync.RWMutex
	tableQueryRequestCache *TableQueryRequestCache
}

var netMqttClient NetMqttClient

func InitNetMqttClient(clientid string, brokerurl string, brokerport string, mqttcert string, mqttkey string, mqttca string) *NetMqttClient {
	initMqttClient.Do(func() {
		netMqttClient = NetMqttClient{
			topics:                 make(map[string]mqtt.MessageHandler),
			clientID:               clientid,
			mainMqttClient:         nil,
			brokerUrl:              brokerurl,
			brokerPort:             brokerport,
			mqttCert:               mqttcert,
			mqttKey:                mqttkey,
			mqttCa:                 mqttca,
			mqttClientMutex:        &sync.RWMutex{},
			mqttWriteMutex:         &sync.Mutex{},
			mqttTopicsMutex:        &sync.RWMutex{},
			tableQueryRequestCache: GetTableQueryRequestCacheInstance(),
		}

		netMqttClient.topics[fmt.Sprintf("nodes/%s/net/tablequery/result", netMqttClient.clientID)] =
			netMqttClient.tableQueryRequestCache.TablequeryResultMqttHandler
		netMqttClient.topics[fmt.Sprintf("nodes/%s/net/subnetwork/result", netMqttClient.clientID)] =
			subnetworkAssignmentMqttHandler

		netMqttClient.runMqttClient(netMqttClient.newClientOptions())
	})
	return &netMqttClient
}

var messageDefaultHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	log.Printf("DEBUG - Received message: %s from topic: %s\n", msg.Payload(), msg.Topic())
}

var subscribeHandlerDispatcher = func(client mqtt.Client, msg mqtt.Message) {
	handlerlist := make([]mqtt.MessageHandler, 0)
	netMqttClient.mqttTopicsMutex.RLock()
	for key, handler := range netMqttClient.topics {
		if strings.Contains(msg.Topic(), key) {
			handlerlist = append(handlerlist, handler)
		}
	}
	netMqttClient.mqttTopicsMutex.RUnlock()
	for _, handler := range handlerlist {
		handler(client, msg)
	}
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	log.Println("Connected to the MQTT broker")

	topicsQosMap := make(map[string]byte)
	netMqttClient.mqttTopicsMutex.RLock()
	for key := range netMqttClient.topics {
		topicsQosMap[key] = 1
	}
	netMqttClient.mqttTopicsMutex.RUnlock()

	//subscribe to all the topics
	tqtoken := client.SubscribeMultiple(topicsQosMap, subscribeHandlerDispatcher)
	tqtoken.Wait()
	log.Printf("Subscribed to topics \n")

}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	logger.ErrorLogger().Printf("Connect lost: %v", err)
}

// newClientOptions builds the client options, reading the certificate files, so a
// reconnect after the worker certificate was renewed uses the new files.
func (netmqtt *NetMqttClient) newClientOptions() *mqtt.ClientOptions {
	opts := mqtt.NewClientOptions()
	if netmqtt.mqttCa == "" {
		// Without a cluster CA (non-gateway setups) keep the plain broker first, as before.
		opts.AddBroker(fmt.Sprintf("tcp://%s:%s", netmqtt.brokerUrl, netmqtt.brokerPort))
	}
	opts.SetClientID(netmqtt.clientID)
	opts.SetUsername("")
	opts.SetPassword("")
	opts.SetDefaultPublishHandler(messageDefaultHandler)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler

	if netmqtt.mqttCert != "" {
		logger.InfoLogger().Printf("MQTT - Configuring TLS")
		cert, err := tls.LoadX509KeyPair(netmqtt.mqttCert, netmqtt.mqttKey)
		logger.InfoLogger().Printf("Cert: %s, Key: %s", netmqtt.mqttCert, netmqtt.mqttKey)
		if err != nil {
			logger.ErrorLogger().Printf("Error loading certificate: %v", err)
		}
		tlsCfg := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
		if netmqtt.mqttCa != "" {
			caPem, err := os.ReadFile(netmqtt.mqttCa)
			if err != nil {
				logger.ErrorLogger().Fatalf("Error reading cluster CA: %v", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(caPem) {
				logger.ErrorLogger().Fatalf("Failed to parse cluster CA PEM")
			}
			tlsCfg.RootCAs = pool
		}
		opts.SetTLSConfig(tlsCfg)
		opts.AddBroker(fmt.Sprintf("tls://%s:%s", netmqtt.brokerUrl, netmqtt.brokerPort))
	}
	return opts
}

func GetNetMqttClient() *NetMqttClient {
	return &netMqttClient
}

func (netmqtt *NetMqttClient) runMqttClient(opts *mqtt.ClientOptions) {
	client := mqtt.NewClient(opts)
	netmqtt.mqttClientMutex.Lock()
	netmqtt.mainMqttClient = client
	netmqtt.mqttClientMutex.Unlock()
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
}

// Initialized reports whether the node registered and the MQTT client exists.
func (netmqtt *NetMqttClient) Initialized() bool {
	if netmqtt.mqttClientMutex == nil {
		return false
	}
	netmqtt.mqttClientMutex.RLock()
	defer netmqtt.mqttClientMutex.RUnlock()
	return netmqtt.mainMqttClient != nil
}

// Reconnect replaces the MQTT client with one built from freshly read certificate
// files and resubscribes to all registered topics
func (netmqtt *NetMqttClient) Reconnect() error {
	opts := netmqtt.newClientOptions()
	opts.SetConnectRetry(true)
	client := mqtt.NewClient(opts)
	netmqtt.mqttClientMutex.Lock()
	previous := netmqtt.mainMqttClient
	netmqtt.mainMqttClient = client
	netmqtt.mqttClientMutex.Unlock()
	previous.Disconnect(250)
	token := client.Connect()
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		return fmt.Errorf("reconnect with the renewed certificate failed: %v", token.Error())
	}
	return nil
}

func (netmqtt *NetMqttClient) client() mqtt.Client {
	netmqtt.mqttClientMutex.RLock()
	defer netmqtt.mqttClientMutex.RUnlock()
	return netmqtt.mainMqttClient
}

func (netmqtt *NetMqttClient) PublishToBroker(topic string, payload string) error {
	netmqtt.mqttWriteMutex.Lock()
	logger.DebugLogger().Printf("MQTT - publish to - %s - the payload - %s", topic, payload)
	token := netmqtt.client().Publish(fmt.Sprintf("nodes/%s/net/%s", netmqtt.clientID, topic), 1, false, payload)
	netmqtt.mqttWriteMutex.Unlock()
	if token.WaitTimeout(time.Second*5) && token.Error() != nil {
		log.Printf("ERROR: MQTT PUBLISH: %s", token.Error())
	}
	return nil
}

func (netmqtt *NetMqttClient) RegisterTopic(topic string, handler mqtt.MessageHandler) {
	netmqtt.mqttTopicsMutex.Lock()
	defer netmqtt.mqttTopicsMutex.Unlock()
	netmqtt.topics[topic] = handler //adding the topic to the global topic list to be handled in case of disconnection
	tqtoken := netmqtt.client().Subscribe(topic, 1, handler)
	tqtoken.WaitTimeout(time.Second * 5)
}

func (netmqtt *NetMqttClient) DeRegisterTopic(topic string) {
	netmqtt.mqttTopicsMutex.Lock()
	defer netmqtt.mqttTopicsMutex.Unlock()
	netmqtt.client().Unsubscribe(topic)
	delete(netmqtt.topics, topic) //removing topic from the topic list in case of disconnection
}
