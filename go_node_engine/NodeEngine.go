package main

import (
	"flag"
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

var clusterAddress = flag.String("a", "localhost", "Address of the cluster orchestrator without port")
var clusterPort = flag.String("p", "10000", "Port of the cluster orchestrator")
var overlayNetwork = flag.Int("n", -1, "Port of the NetManager component, if any. This enables the overlay network across nodes")
var UnikernelSupport = flag.Bool("u", false, "Set to enable Unikernel support")
var LogDirectory = flag.String("logs", "/tmp", "Directory for application's logs")

const MONITORING_CYCLE = time.Second * 2

func main() {
	flag.Parse()

	//set log directory
	model.GetNodeInfo().SetLogDirectory(*LogDirectory)

	//connect to container runtime
	runtime := virtualization.GetContainerdClient()
	defer runtime.StopContainerdClient()

	if *UnikernelSupport {
		unikernelRuntime := virtualization.GetUnikernelRuntime()
		defer unikernelRuntime.StopUnikernelRuntime()
	}
	//hadshake with the cluster orchestrator to get mqtt port and node id
	handshakeResult := clusterHandshake()

	//enable overlay network if required
	if *overlayNetwork > 0 {
		model.EnableOverlay(*overlayNetwork)
		err := requests.RegisterSelfToNetworkComponent()
		if err != nil {
			logger.ErrorLogger().Fatalf("Unable to register to NetManager: %v", err)
		}
	}

	//binding the node MQTT client
	mqtt.InitMqtt(handshakeResult.NodeId, *clusterAddress, handshakeResult.MqttPort)

	//starting node status background job.
	jobs.NodeStatusUpdater(MONITORING_CYCLE, mqtt.ReportNodeInformation)
	//starting container resources background monitor.
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
	logger.InfoLogger().Printf("INIT: Starting handshake with cluster orhcestrator %s:%s", *clusterAddress, *clusterPort)
	node := model.GetNodeInfo()
	logger.InfoLogger().Printf("Node Statistics: \n__________________")
	logger.InfoLogger().Printf("CPU Cores: %d", node.CpuCores)
	logger.InfoLogger().Printf("CPU Usage: %f", node.CpuUsage)
	logger.InfoLogger().Printf("Mem Usage: %f", node.MemoryUsed)
	logger.InfoLogger().Printf("GPU Driver: %s", node.GpuDriver)
	logger.InfoLogger().Printf("\n________________")
	clusterReponse := requests.ClusterHandshake(*clusterAddress, *clusterPort)
	logger.InfoLogger().Printf("Got cluster response with MQTT port %s and node ID %s", clusterReponse.MqttPort, clusterReponse.NodeId)

	model.SetNodeId(clusterReponse.NodeId)
	return clusterReponse
}
