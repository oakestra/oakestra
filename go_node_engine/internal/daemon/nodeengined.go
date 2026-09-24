package main

import (
	"fmt"
	"go_node_engine/addons"
	"go_node_engine/cmd"
	"go_node_engine/config"
	"go_node_engine/csi"
	"go_node_engine/internal/store"
	"go_node_engine/jobs"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/mqtt"
	p2pnode "go_node_engine/p2p"
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

	var err error
	configs, err = config.Read()
	if err != nil {
		logger.ErrorLogger().Fatal(err)
	}

	// set log directory
	model.GetNodeInfo().SetLogDirectory(configs.AppLogs)

	// set cluster address
	model.GetNodeInfo().SetClusterAddress(configs.ClusterAddress)

	// Initialize virtualization runtimes
	runtimeManager, err := virtualization.NewRuntimeManager()
	if err != nil {
		logger.ErrorLogger().Fatal(err)
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
		logger.InfoLogger().Printf("CSI driver available: %s (%s)", driver.Name, driver.Endpoint)
	}
	defer csiReg.StopAll()

	//Startup Addons
	for _, addon := range configs.Addons {
		if addon.Active {
			logger.InfoLogger().Printf("Startup addon: %s", addon.Name)
			addons.StartupAddon(model.AddonType(addon.Name), addon.Config)
		}
	}

	p2pMode := configs.NodeModeOrDefault() == config.MODE_P2P

	// Cluster mode: handshake with the cluster orchestrator to get mqtt port and node id.
	// P2P mode: no cluster — use the stable NodeUUID and skip the handshake.
	var handshakeResult requests.HandshakeAnswer
	if p2pMode {
		logger.InfoLogger().Printf("P2P mode 🟢 — skipping cluster handshake, node UUID %s", configs.NodeUUID)
		model.SetNodeId(configs.NodeUUID)

		// Enrollment / genesis: process one-shot join params BEFORE the overlay
		// starts, so the gossip PSK is available for the NetManager registration.
		if configs.P2P.HasPendingEnroll() {
			if configs.P2P.Pending.Init {
				if _, err := p2pnode.Genesis(configs.NodeUUID); err != nil {
					logger.ErrorLogger().Fatalf("P2P genesis failed: %v", err)
				}
			} else {
				if _, err := p2pnode.Join(configs.NodeUUID, configs.P2P.Pending); err != nil {
					logger.ErrorLogger().Fatalf("P2P enrollment failed: %v", err)
				}
			}
			// join params are single-use: clear them after success
			configs.P2P.Pending = config.PendingEnroll{}
			if err := config.Write(configs); err != nil {
				logger.ErrorLogger().Printf("WARN: could not clear enrollment params: %v", err)
			}
		}
		if _, ok := p2pnode.LoadCreds(); !ok {
			// Bare --p2p on a fresh node: bootstrap a new network (same as --init).
			logger.InfoLogger().Printf("P2P: no credentials found — bootstrapping a new network")
			if _, err := p2pnode.Genesis(configs.NodeUUID); err != nil {
				logger.ErrorLogger().Fatalf("P2P genesis failed: %v", err)
			}
		}
		// Every member runs the enrollment endpoint — any member can onboard.
		go func() {
			enrollPort := configs.P2P.EnrollPort
			if enrollPort == 0 {
				enrollPort = 50106
			}
			if err := p2pnode.ServeEnrollment(configs.NodeUUID, enrollPort, configs.P2P.GossipPort); err != nil {
				logger.ErrorLogger().Printf("P2P enrollment endpoint failed: %v", err)
			}
		}()
	} else {
		handshakeResult = clusterHandshake()
	}

	// enable overlay network if required
	switch configs.OverlayNetwork {
	case config.AutoOakNetwork:
		logger.InfoLogger().Printf("Looking for local NetManager socket.")
		_ = exec.Command("systemctl", "enable", "netmanager").Run() // survive reboot
		cmd := exec.Command("systemctl", "start", "netmanager")
		_ = cmd.Run()
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
		logger.InfoLogger().Printf("Overlay network enabled 🟢")
		// wait for systemctl to start the netmanager service
		logger.InfoLogger().Printf("Waiting for NetManager to start...")
		time.Sleep(5 * time.Second)
		err := requests.RegisterSelfToNetworkComponent(configs)
		if err != nil {
			logger.ErrorLogger().Fatalf("Error registering to NetManager: %v", err)
		}
	}

	// Cluster mode uses MQTT for the control plane; p2p mode uses gossip + the node
	// control API.
	if !p2pMode {
		// binding the node MQTT client
		mqtt.InitMqtt(handshakeResult.NodeId, configs.ClusterAddress, handshakeResult.MqttPort, configs.CertFile, configs.KeyFile, runtimeManager)

		// No local restore after a reboot: in cluster mode the cluster owns placement
		// and re-deploys the instances it marked FAILED while this node was offline.

		// starting node status background job.
		jobs.NodeStatusUpdater(MONITORING_CYCLE, mqtt.ReportNodeInformation)
		// starting container resources background monitor.
		jobs.StartServicesMonitoring(runtimeManager, MONITORING_CYCLE, mqtt.ReportServiceResources)
	} else {
		// P2P mode: start the controller — coordinator (bid rounds), host (assigns),
		// custodian (failover + PENDING retry) and observability endpoint.
		var ctrl *p2pnode.Controller
		statusFn := func(svc model.Service) {
			if ctrl != nil {
				ctrl.ReportStatus(svc)
			}
		}
		deployFn := func(svc model.Service) error {
			rt := runtimeManager.GetRuntime(model.RuntimeType(svc.Runtime))
			return rt.Deploy(svc, statusFn)
		}
		undeployFn := func(sname string, instance int) error {
			records, _ := store.Load()
			for _, rec := range records {
				if rec.Service.Sname == sname && rec.Service.Instance == instance {
					rt := runtimeManager.GetRuntime(model.RuntimeType(rec.Service.Runtime))
					return rt.Undeploy(sname, instance)
				}
			}
			return fmt.Errorf("service %s.%d is not deployed on this node", sname, instance)
		}

		var err error
		ctrl, err = p2pnode.NewController(configs, deployFn, undeployFn)
		if err != nil {
			logger.ErrorLogger().Fatalf("P2P controller startup failed: %v", err)
		}
		go func() {
			if err := ctrl.Serve(); err != nil {
				logger.ErrorLogger().Fatalf("P2P control API failed: %v", err)
			}
		}()

		// Keep node resource stats fresh for bid evaluation (no MQTT reporting).
		jobs.NodeStatusUpdater(MONITORING_CYCLE, func(model.Node) {})

		// Gossip our cert fingerprint so peers can verify our mTLS identity.
		go ctrl.AnnounceRoster()

		// Reboot recovery with the generation rule: reconcile BEFORE reclaiming.
		go ctrl.ReconcileP2P()
	}

	// catch SIGETRM or SIGINTERRUPT
	termination := make(chan os.Signal, 1)
	signal.Notify(termination, syscall.SIGTERM, syscall.SIGINT)
	ossignal := <-termination
	logger.InfoLogger().Printf("Terminating the NodeEngine, signal:%v", ossignal)
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
