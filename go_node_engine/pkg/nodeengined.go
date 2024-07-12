package main

import (
	"encoding/json"
	"fmt"
	"go_node_engine/cmd"
	"go_node_engine/jobs"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/mqtt"
	"go_node_engine/requests"
	"go_node_engine/virtualization"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const MONITORING_CYCLE = time.Second * 2

var configs cmd.ConfFile

func main() {
	configs = readConf()

	// set log directory
	model.GetNodeInfo().SetLogDirectory(configs.AppLogs)

	// connect to container runtime
	runtime := virtualization.GetContainerdClient()
	defer runtime.StopContainerdClient()

	if configs.UnikernelSupport {
		unikernelRuntime := virtualization.GetUnikernelRuntime()
		defer unikernelRuntime.StopUnikernelRuntime()
	}
	// hadshake with the cluster orchestrator to get mqtt port and node id
	handshakeResult := clusterHandshake()

	// enable overlay network if required
	if configs.NetPort > 0 {
		model.EnableOverlay(configs.NetPort)
	} else {
		if configs.OverlayNetwork == cmd.DEFAULT_CNI {
			logger.InfoLogger().Printf("Looking for local NetManager socket.")
			err := requests.RegisterSelfToNetworkComponent()
			if err != nil {
				//fatal error
				logger.ErrorLogger().Fatalf("Error registering to NetManager: %v", err)
			} else {
				model.EnableOverlay(0)
			}
		} else {
			logger.InfoLogger().Printf("Overlay network disabled 🟠")
		}
	}

	// binding the node MQTT client
	mqtt.InitMqtt(handshakeResult.NodeId, configs.ClusterAddress, handshakeResult.MqttPort)

	// starting node status background job.
	jobs.NodeStatusUpdater(MONITORING_CYCLE, mqtt.ReportNodeInformation)
	// starting container resources background monitor.
	jobs.StartServicesMonitoring(MONITORING_CYCLE, mqtt.ReportServiceResources)

	// catch SIGETRM or SIGINTERRUPT
	termination := make(chan os.Signal, 1)
	signal.Notify(termination, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
	select {
	case ossignal := <-termination:
		logger.InfoLogger().Printf("Terminating the NodeEngine, signal:%v", ossignal)
	}
}

func clusterHandshake() requests.HandshakeAnswer {
	logger.InfoLogger().Printf("INIT: Starting handshake with cluster orchestrator %s:%d", configs.ClusterAddress, configs.ClusterPort)
	node := model.GetNodeInfo()
	logger.InfoLogger().Printf("Node Statistics: \n__________________")
	logger.InfoLogger().Printf("CPU Cores: %d", node.CpuCores)
	logger.InfoLogger().Printf("CPU Usage: %f", node.CpuUsage)
	logger.InfoLogger().Printf("Mem Usage: %f", node.MemoryUsed)
	logger.InfoLogger().Printf("GPU Driver: %s", node.GpuDriver)
	logger.InfoLogger().Printf("\n________________")
	clusterReponse := requests.ClusterHandshake(configs.ClusterAddress, configs.ClusterPort)
	logger.InfoLogger().Printf("Got cluster response with MQTT port %s and node ID %s", clusterReponse.MqttPort, clusterReponse.NodeId)

	model.SetNodeId(clusterReponse.NodeId)
	return clusterReponse
}

func readConf() cmd.ConfFile {
	confFile, err := os.Open("/etc/oakestra/conf.json")
	cfg := cmd.ConfFile{}
	if err != nil {
		logger.ErrorLogger().Fatalf("Error reading configuration: %v\n, resetting the file", err)
	}
	defer confFile.Close()

	//read cluster configuration
	buffer := make([]byte, 2048)
	n, err := confFile.Read(buffer)
	if err != nil {
		logger.ErrorLogger().Fatalf("Error reading configuration: %v\n, resetting the file", err)
	}
	err = json.Unmarshal(buffer[:n], &cfg)
	if err != nil {
		fmt.Printf("Error reading configuration: %v\n, resetting the file", err)
	}
	return cfg
}
