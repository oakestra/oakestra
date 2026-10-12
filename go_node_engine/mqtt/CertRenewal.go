package mqtt

import (
	"go_node_engine/config"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/requests"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// The cluster asks for a renewal when the certificate is close to expiry or was issued
// by a replaced intermediate
const minRenewalInterval = 10 * time.Minute

var renewalMu sync.Mutex
var lastRenewal time.Time

func renewCertHandler(client mqtt.Client, msg mqtt.Message) {
	// Renewal reconnects this client, so it must not run inside the message handler.
	go renewWorkerCert()
}

func renewWorkerCert() {
	if !renewalMu.TryLock() {
		return
	}
	defer renewalMu.Unlock()
	if time.Since(lastRenewal) < minRenewalInterval {
		return
	}
	lastRenewal = time.Now()

	cfg, err := config.Read()
	if err != nil {
		logger.ErrorLogger().Printf("Worker certificate renewal: could not read config: %v", err)
		return
	}
	logger.InfoLogger().Printf("Cluster requested a worker certificate renewal")
	if err := requests.RenewWorkerCert(cfg); err != nil {
		logger.ErrorLogger().Printf("Worker certificate renewal failed, keeping the current certificate: %v", err)
		return
	}
	logger.InfoLogger().Printf("Worker certificate renewed; reconnecting to the cluster")
	reconnectMqtt()
	// NetManager uses the same certificate files for its own MQTT connection.
	if model.GetNodeInfo().Overlay {
		if err := requests.ReconnectNetManagerMqtt(); err != nil {
			logger.ErrorLogger().Printf("NetManager could not reconnect with the renewed certificate: %v", err)
		}
	}
}
