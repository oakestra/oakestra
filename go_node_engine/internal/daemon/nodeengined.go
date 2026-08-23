package main

import (
	"go_node_engine/addons"
	"go_node_engine/cmd"
	"go_node_engine/config"
	"go_node_engine/csi"
	"go_node_engine/jobs"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/mqtt"
	"go_node_engine/requests"
	"go_node_engine/virtualization"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/containers/storage/pkg/reexec"
)

const MONITORING_CYCLE = time.Second * 2

var configs config.ConfFile

func main() {
	// The OCI libraries support performing certain container-related tasks in a sandboxed child process.
	// For this to work, binaries using these libraries must use the reexec.Init() hook before doing anything else.
	// This way, the library can take over execution in the mentioned child processes and will return true in that case.
	if reexec.Init() {
		return
	}

	configManager := config.GetConfFileManager()
	var err error
	configs, err = configManager.Get()
	if err != nil {
		logger.FatalErrorLogger("%v", err)
	}

	// set log directory
	model.GetNodeInfo().SetLogDirectory(configs.AppLogs)

	// set cluster address
	model.GetNodeInfo().SetClusterAddress(configs.ClusterAddress)

	// Initialize virtualization runtimes
	runtimeManager, err := virtualization.NewRuntimeManager()
	if err != nil {
		logger.FatalErrorLogger("%v", err)
	}
	for _, virt := range configs.Virtualizations {
		if virt.Active {
			rt := runtimeManager.GetRuntime(model.RuntimeType(virt.Runtime))
			defer rt.Stop()
			model.GetNodeInfo().AddSupportedTechnology(model.RuntimeType(virt.Runtime))
		}
	}

	// Initialize and probe CSI plugins listed in the node configuration.
	// Successfully probed plugins are registered and advertised to the cluster.
	csiReg := csi.GetRegistry()
	csiReg.InitFromConfig(configs)
	for _, driver := range csiReg.List() {
		model.GetNodeInfo().AddCSIDriver(driver)
		logger.InfoLogger("CSI driver available: %s (%s)", driver.Name, driver.Endpoint)
	}
	defer csiReg.StopAll()

	//Startup Addons
	for _, addon := range configs.Addons {
		if addon.Active {
			logger.InfoLogger("Startup addon: %s", addon.Name)
			addons.StartupAddon(model.AddonType(addon.Name), addon.Config)
		}
	}

	// hadshake with the cluster orchestrator to get mqtt port and node id
	handshakeResult := clusterHandshake()

	// enable overlay network if required
	switch configs.OverlayNetwork {
	case config.AUTO_OAK_NETWORK:
		logger.InfoLogger("Looking for local NetManager socket.")
		cmd := exec.Command("systemctl", "start", "netmanager")
		_ = cmd.Run()
		model.EnableOverlay()
	case cmd.DISABLE_NETWORK:
		logger.InfoLogger("Overlay network disabled 🟠")
	default:
		if strings.Contains(configs.OverlayNetwork, "custom:") {
			netPath := strings.Split(configs.OverlayNetwork, ":")
			model.GetNodeInfo().SetOverlaySocket(netPath[1])
			model.EnableOverlay()
		} else {
			logger.InfoLogger("Invalid overlay network detected. Network disabled 🟠")
		}
	}
	if model.GetNodeInfo().Overlay {
		logger.InfoLogger("Overlay network enabled 🟢")
		// wait for systemctl to start the netmanager service
		logger.InfoLogger("Waiting for NetManager to start...")
		time.Sleep(5 * time.Second)
		err := requests.RegisterSelfToNetworkComponent()
		if err != nil {
			logger.FatalErrorLogger("Error registering to NetManager: %v", err)
		}
	}

	// binding the node MQTT client
	mqtt.InitMqtt(handshakeResult.NodeId, configs.ClusterAddress, handshakeResult.MqttPort, configs.CertFile, configs.KeyFile, runtimeManager)

	// starting node status background job.
	jobs.NodeStatusUpdater(MONITORING_CYCLE, mqtt.ReportNodeInformation)
	// starting container resources background monitor.
	jobs.StartServicesMonitoring(runtimeManager, MONITORING_CYCLE, mqtt.ReportServiceResources)

	// catch SIGETRM or SIGINTERRUPT
	termination := make(chan os.Signal, 1)
	signal.Notify(termination, syscall.SIGTERM, syscall.SIGINT)
	ossignal := <-termination
	logger.InfoLogger("Terminating the NodeEngine, signal:%v", ossignal)
}

func clusterHandshake() requests.HandshakeAnswer {
	logger.InfoLogger("INIT: Starting handshake with cluster orchestrator %s:%d", configs.ClusterAddress, configs.ClusterPort)
	node := model.GetNodeInfo()
	logger.InfoLogger("Node Statistics: \n__________________")
	logger.InfoLogger("CPU Cores: %d", node.CpuCores)
	logger.InfoLogger("CPU Usage: %f", node.CpuUsage)
	logger.InfoLogger("Mem Usage: %f", node.MemoryUsed)
	logger.InfoLogger("GPU Driver: %s", node.GpuDriver)
	logger.InfoLogger("\n________________")
	clusterReponse := requests.ClusterHandshake(configs.ClusterAddress, configs.ClusterPort)
	logger.InfoLogger("Got cluster response with MQTT port %s and node ID %s", clusterReponse.MqttPort, clusterReponse.NodeId)

	model.SetNodeId(clusterReponse.NodeId)
	return clusterReponse
}
