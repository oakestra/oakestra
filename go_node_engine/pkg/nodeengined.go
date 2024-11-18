package main

import (
	"go_node_engine/addons"
	"go_node_engine/cmd"
	"go_node_engine/config"
	"go_node_engine/jobs"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/mqtt"
	"go_node_engine/requests"
	"go_node_engine/virtualization"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const MONITORING_CYCLE = time.Second * 2

var configs config.ConfFile

func main() {
	configManager := config.GetConfFileManager()
	configs, err := configManager.Get()
	if err != nil {
		logger.ErrorLogger().Fatal(err)
	}

	// set log directory
	model.GetNodeInfo().SetLogDirectory(configs.AppLogs)

	// set cluster address
	model.GetNodeInfo().SetClusterAddress(configs.ClusterAddress)

	//Set Virtualization Runtimes
	for _, virt := range configs.Virtualizations {
		if virt.Active {
			rt := virtualization.GetRuntime(model.RuntimeType(virt.Runtime))
			defer rt.Stop()
		}
	}

	//Startup Addons
	for _, addon := range configs.Addons {
		if addon.Active {
			addons.StartupAddon(model.AddonType(addon.Name), addon.Config)
		}
	}

	// hadshake with the cluster orchestrator to get mqtt port and node id
	handshakeResult := clusterHandshake()

	// enable overlay network if required
	switch configs.OverlayNetwork {
	case config.DEFAULT_CNI:
		logger.InfoLogger().Printf("Looking for local NetManager socket.")
		model.EnableOverlay()
	case cmd.DISABLE_NETWORK:
		logger.InfoLogger().Printf("Overlay network disabled 🟠")
	default:
		if strings.Contains(configs.OverlayNetwork, "custom:") {
			netPath := strings.Split(configs.OverlayNetwork, ":")
			model.GetNodeInfo().SetOverlaySocket(netPath[1])
			model.EnableOverlay()
		} else {
			logger.InfoLogger().Printf("Invalid overlay network detected. Network disabled 🟠")
		}
	}
	if model.GetNodeInfo().Overlay {
		err := requests.RegisterSelfToNetworkComponent()
		if err != nil {
			//fatal error
			logger.ErrorLogger().Fatalf("Error registering to NetManager: %v", err)
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
	signal.Notify(termination, syscall.SIGTERM, syscall.SIGINT)
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
