package requests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"io/ioutil"
	"net/http"
)

type HandshakeAnswer struct {
	MqttPort string `json:"MQTT_BROKER_PORT"`
	NodeId   string `json:"id"`
}

func ClusterHandshake(address string, port string) HandshakeAnswer {
	data, err := json.Marshal(model.GetNodeInfo())
	if err != nil {
		logger.ErrorLogger().Fatalf("Handshake failed, json encoding problem, %v", err)
	}
	jsonbody := bytes.NewBuffer(data)
	resp, err := http.Post(fmt.Sprintf("http://%s:%s/api/node/register", address, port), "application/json", jsonbody)
	if err != nil {
		logger.ErrorLogger().Fatalf("Handshake failed, %v", err)
	}
	if resp.StatusCode != 200 {
		logger.ErrorLogger().Fatalf("Handshake failed with error code %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	handhsakeanswer := HandshakeAnswer{}
	responseBytes, err := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(responseBytes, &handhsakeanswer)
	if err != nil {
		logger.ErrorLogger().Fatalf("Handshake failed, %v", err)
	}
	return handhsakeanswer
}
