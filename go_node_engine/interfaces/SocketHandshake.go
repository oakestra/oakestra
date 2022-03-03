package interfaces

import (
	"encoding/json"
	"fmt"
	"github.com/wedeploy/gosocketio"
	"github.com/wedeploy/gosocketio/websocket"
	"go_node_engine/model"
	"log"
	"net/url"
	"time"
)

type socketClient struct {
	client    *gosocketio.Namespace
	connected bool
	result    chan sc2Answer
}

type sc2Answer struct {
	MqttPort string `json:"MQTT_BROKER_PORT"`
	NodeId   string `json:"id"`
}

func DoHandshake(address string, port int) sc2Answer {
	parsedurl, _ := url.Parse(fmt.Sprintf("%s:%d", address, port))
	c, err := gosocketio.Connect(*parsedurl, websocket.NewTransport())
	if err != nil {
		log.Fatal(err)
	}
	namespace, err := c.Of("/init")
	if err != nil {
		log.Fatal(err)
	}
	socketclient := socketClient{
		client:    namespace,
		connected: false,
		result:    make(chan sc2Answer),
	}
	socketclient.registerEvents()
	select {
	case res := <-socketclient.result:
		return res
	case <-time.After(5 * time.Second):
		log.Fatal("Handshake timeout")
	}
	return sc2Answer{}
}

func (c *socketClient) registerEvents() {
	err := c.client.On("sc1", c.sc1Handler)
	if err != nil {
		log.Fatalf("Unable to complete the handshake %v", err)
	}
	err = c.client.On("sc2", c.sc2Handler)
	if err != nil {
		log.Fatalf("Unable to complete the handshake %v", err)
	}
}

func (s *socketClient) sc1Handler(conn *gosocketio.Client) {
	nodeInfo := model.GetNodeInfo()
	jsonInfo, err := json.Marshal(nodeInfo)
	if err != nil {
		log.Print(nodeInfo)
		log.Fatalf("Unable to complete the handshake %v", err)
	}
	time.Sleep(time.Second) // Wait to avoid Race Condition between sending first message and receiving connection establishment
	conn.Emit("cs1", string(jsonInfo))

}

func (s *socketClient) sc2Handler(conn *gosocketio.Client, answer sc2Answer) {
	s.result <- answer
	_ = conn.Close()
}
