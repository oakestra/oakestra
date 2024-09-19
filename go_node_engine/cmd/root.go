package cmd

import (
	"go_node_engine/addons/flops"
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

	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:   "NodeEngine",
		Short: "Start a NoderEngine",
		Long:  `Start a New Oakestra Worker Node`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return startNodeEngine()
		},
	}
	clusterAddress   string
	clusterPort      int
	overlayNetwork   int
	unikernelSupport bool
	logDirectory     string
	// Addons
	imageBuilder        bool
	flopsLearnerSupport bool
)

// MONITORING_CYCLE defines the interval at which the system should perform monitoring tasks.
const MONITORING_CYCLE = time.Second * 2

// Execute is the entry point of the NodeEngine
func Execute() error {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	return rootCmd.Execute()
}

func init() {
	rootCmd.Flags().StringVarP(&clusterAddress, "clusterAddr", "a", "localhost", "Address of the cluster orchestrator without port")
	rootCmd.Flags().IntVarP(&clusterPort, "clusterPort", "p", 10100, "Port of the cluster orchestrator")
	rootCmd.Flags().IntVarP(&overlayNetwork, "netmanagerPort", "n", 6000, "Port of the NetManager component, if any. This enables the overlay network across nodes. Use -1 to disable Overlay Network Mode.")
	rootCmd.Flags().BoolVarP(&unikernelSupport, "unikernel", "u", false, "Enable Unikernel support. [qemu/kvm required]")
	rootCmd.Flags().StringVarP(&logDirectory, "logs", "l", "/tmp", "Directory for application's logs")
	// Addons
	rootCmd.Flags().BoolVar(&imageBuilder, "image-builder", false, "Checks if the host has QEMU (apt's qemu-user-static) installed for building multi-platform images.")
	rootCmd.Flags().BoolVar(&flopsLearnerSupport, "flops-learner", false, "Enables the ML-data-server sidecar for data collection for FLOps learners.")
}

func startNodeEngine() error {
	// set log directory
	model.GetNodeInfo().SetLogDirectory(logDirectory)

	// connect to container runtime
	runtime := virtualization.GetContainerdClient()
	defer runtime.StopContainerdClient()

	if unikernelSupport {
		unikernelRuntime := virtualization.GetUnikernelRuntime()
		defer unikernelRuntime.StopUnikernelRuntime()
	}

	if imageBuilder {
		cmd := exec.Command("dpkg", "-s", "qemu-user-static")
		output, err := cmd.Output()
		if err != nil || !strings.Contains(string(output), "ok installed") {
			logger.ErrorLogger().Fatalf("Unable to find qemu-user-static apt package for multi-platform image-builder: %v\n", err)
		}
		model.GetNodeInfo().AddSupportedAddons(model.IMAGE_BUILDER)
	}

	if flopsLearnerSupport {
		model.GetNodeInfo().AddSupportedAddons(model.FLOPS_LEARNER)
		flops.HandleFLOpsDataManager()
	}

	// hadshake with the cluster orchestrator to get mqtt port and node id
	handshakeResult := clusterHandshake()

	// enable overlay network if required
	if overlayNetwork > 0 {
		model.EnableOverlay(overlayNetwork)
		err := requests.RegisterSelfToNetworkComponent()
		if err != nil {
			logger.ErrorLogger().Fatalf("Unable to register to NetManager: %v", err)
		}
	}

	// binding the node MQTT client
	mqtt.InitMqtt(handshakeResult.NodeId, clusterAddress, handshakeResult.MqttPort)

	// starting node status background job.
	jobs.NodeStatusUpdater(MONITORING_CYCLE, mqtt.ReportNodeInformation)
	// starting container resources background monitor.
	jobs.StartServicesMonitoring(MONITORING_CYCLE, mqtt.ReportServiceResources)

	// catch SIGETRM or SIGINTERRUPT
	termination := make(chan os.Signal, 1)
	// SIGKILL cannot be trapped, using SIGTERM instead
	signal.Notify(termination, syscall.SIGTERM, syscall.SIGINT)
	select {
	case ossignal := <-termination:
		logger.InfoLogger().Printf("Terminating the NodeEngine, signal:%v", ossignal)
	}

	return nil
}

func clusterHandshake() requests.HandshakeAnswer {
	logger.InfoLogger().Printf("INIT: Starting handshake with cluster orchestrator %s:%d", clusterAddress, clusterPort)
	node := model.GetNodeInfo()
	logger.InfoLogger().Printf("Node Statistics: \n__________________")
	logger.InfoLogger().Printf("CPU Cores: %d", node.CpuCores)
	logger.InfoLogger().Printf("CPU Usage: %f", node.CpuUsage)
	logger.InfoLogger().Printf("Mem Usage: %f", node.MemoryUsed)
	logger.InfoLogger().Printf("GPU Driver: %s", node.GpuDriver)
	logger.InfoLogger().Printf("\n________________")
	clusterReponse := requests.ClusterHandshake(clusterAddress, clusterPort)
	logger.InfoLogger().Printf("Got cluster response with MQTT port %s and node ID %s", clusterReponse.MqttPort, clusterReponse.NodeId)

	model.SetNodeId(clusterReponse.NodeId)
	return clusterReponse
}
