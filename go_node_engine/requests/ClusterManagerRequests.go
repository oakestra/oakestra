package requests

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"go_node_engine/config"
	"go_node_engine/logger"
	"go_node_engine/model"
	"io"
	"net/http"
	"os"
	"time"
)

// HandshakeAnswer is the struct that describes the handshake answer between the nodes
type HandshakeAnswer struct {
	MqttPort string `json:"MQTT_BROKER_PORT"`
	NodeId   string `json:"id"`
}

// ClusterHandshake sends a handshake request to the cluster manager
func ClusterHandshake(address string, port int) HandshakeAnswer {
	data, err := json.Marshal(model.GetNodeInfo())
	if err != nil {
		logger.ErrorLogger().Fatalf("Handshake failed, json encoding problem, %v", err)
	}
	jsonbody := bytes.NewBuffer(data)

	cfg, err := config.GetConfFileManager().Get()
	if err != nil {
		logger.ErrorLogger().Fatalf("Could not get config")
	}

	var resp *http.Response
	proto := "http"
	client := http.DefaultClient
	if cfg.ClusterSSL {
		proto = "https"
		if cfg.WorkerCertFile != "" && cfg.WorkerKeyFile != "" {
			cert, err := tls.LoadX509KeyPair(cfg.WorkerCertFile, cfg.WorkerKeyFile)
			if err != nil {
				logger.ErrorLogger().Fatalf("Failed to load worker cert: %v", err)
			}
			tlsConfig := &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
			}
			// Server trust for the cluster gateway's (separate, public) server
			// cert, per ClusterGatewayTrust:
			//   "system"   -> RootCAs nil (OS trust store; the default)
			//   "insecure" -> skip verification
			//   <path>     -> pin that CA bundle
			//   ""         -> fall back to the cluster CA file (internal CA)
			switch {
			case cfg.ClusterGatewayTrust == "system":
				// leave RootCAs nil
			case cfg.ClusterGatewayTrust == "insecure":
				tlsConfig.InsecureSkipVerify = true // #nosec G402 -- operator opt-in
			default:
				trustFile := cfg.ClusterGatewayTrust
				if trustFile == "" {
					trustFile = cfg.ClusterCaFile
				}
				if trustFile != "" {
					caPem, err := os.ReadFile(trustFile)
					if err != nil {
						logger.ErrorLogger().Fatalf("Failed to read gateway trust file %s: %v", trustFile, err)
					}
					pool := x509.NewCertPool()
					if !pool.AppendCertsFromPEM(caPem) {
						logger.ErrorLogger().Fatalf("Failed to parse gateway trust PEM %s", trustFile)
					}
					tlsConfig.RootCAs = pool
				}
			}
			client = &http.Client{
				Transport: &http.Transport{TLSClientConfig: tlsConfig},
				Timeout:   30 * time.Second,
			}
		}
	}
	resp, err = client.Post(fmt.Sprintf("%s://%s:%d/api/node/register", proto, address, port), "application/json", jsonbody)

	if err != nil {
		logger.ErrorLogger().Fatalf("Handshake failed, %v", err)
	}
	if resp.StatusCode != 200 {
		logger.ErrorLogger().Fatalf("Handshake failed with error code %d", resp.StatusCode)
	}
	//defer resp.Body.Close()
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.ErrorLogger().Fatalf("Handshake failed, %v", err)
		}
	}()

	handhsakeanswer := HandshakeAnswer{}
	responseBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.ErrorLogger().Fatalf("Handshake failed, %v", err)
	}
	err = json.Unmarshal(responseBytes, &handhsakeanswer)
	if err != nil {
		logger.ErrorLogger().Fatalf("Handshake failed, %v", err)
	}
	return handhsakeanswer
}
