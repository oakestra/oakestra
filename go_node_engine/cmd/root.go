package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path"

	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:   "NodeEngine",
		Short: "Start a NoderEngine",
		Long:  `Start a New Oakestra Worker Node`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nodeEngineDaemonManager()

		},
	}
	clusterAddress string
	clusterPort    int
	clusterSSL     bool
	detatched      bool
	// Addons
	imageBuilder        bool
	flopsLearnerSupport bool
	// P2P mode
	p2pMode   bool
	p2pInit   bool
	p2pJoin   string
	p2pToken  string
	p2pCaHash string
)

var CONF_DIR = path.Join("/etc", "oakestra")
var CONF_FILE = path.Join(CONF_DIR, "conf.json")
var DISABLE_NETWORK = "disabled"

// Execute is the entry point of the NodeEngine
func Execute() error {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	return rootCmd.Execute()
}

func init() {
	rootCmd.Flags().StringVarP(&clusterAddress, "clusterAddr", "a", "", "Custom address of the cluster orchestrator without port")
	rootCmd.Flags().IntVarP(&clusterPort, "clusterPort", "p", 10100, "Port of the cluster orchestrator")
	rootCmd.Flags().BoolVarP(&detatched, "detatch", "d", false, "Run the NodeEngine in the background (daemon mode)")
	// Addons
	rootCmd.Flags().BoolVar(&imageBuilder, "image-builder", false, "Checks if the host has QEMU (apt's qemu-user-static) installed for building multi-platform images.")
	rootCmd.Flags().BoolVar(&flopsLearnerSupport, "flops-learner", false, "Enables the ML-data-server sidecar for data collection for FLOps learners.")
	// P2P mode
	rootCmd.Flags().BoolVar(&p2pMode, "p2p", false, "Start the worker in peer-to-peer mode (no cluster orchestrator)")
	rootCmd.Flags().BoolVar(&p2pInit, "init", false, "Genesis: create a brand-new p2p network on this node")
	rootCmd.Flags().StringVar(&p2pJoin, "join", "", "P2P enrollment bootstrap address ip:port (from 'addp2p')")
	rootCmd.Flags().StringVar(&p2pToken, "token", "", "P2P single-use join token")
	rootCmd.Flags().StringVar(&p2pCaHash, "ca-hash", "", "P2P trust-anchor pin (sha256:...) verified on first contact")
}

func nodeEngineDaemonManager() error {
	if _, err := os.Stat(CONF_FILE); err != nil {
		err := defaultConfig()
		if err != nil {
			return err
		}
	}

	if p2pMode {
		// P2P mode: persist mode + join params and skip cluster-address handling
		if err := applyP2PStartup(p2pInit, p2pJoin, p2pToken, p2pCaHash); err != nil {
			return err
		}
	}

	if clusterAddress != "" {
		// set new cluster address if users selected a custom one
		err := configAddress(clusterAddress)
		if err != nil {
			return err
		}
	}

	if clusterPort != 10100 {
		// set new cluster port if users selected a custom one
		err := configPort(clusterPort)
		if err != nil {
			return err
		}
	}

	if clusterSSL {
		// set SSL cluster handshake
		err := configSSL(clusterSSL)
		if err != nil {
			return err
		}
	}

	if certFile != "" || keyFile != "" {
		// set Mqtt auth parameters
		err := setMqttAuth()
		if err != nil {
			return err
		}
	}

	// enable at boot so the daemon (and its persisted deployments) survive a reboot
	_ = exec.Command("systemctl", "enable", "nodeengine").Run()

	// start the node engine daemon systemctl daemon
	cmd := exec.Command("systemctl", "start", "nodeengine")
	if err := cmd.Run(); err != nil {
		return err
	}

	fmt.Println("NodeEngine started  🟢")
	if !detatched {
		return attach()
	}

	return nil
}

func attach() error {
	// Show only what the freshly started daemon writes from now on.
	return followLog(nodeEngineLogPath, 0, func() {
		fmt.Println("\nExiting...")
		stopNodeEngine()
	})
}
