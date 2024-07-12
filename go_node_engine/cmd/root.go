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
	clusterAddress   string
	clusterPort      int
	overlayNetwork   int
	unikernelSupport bool
	logDirectory     string
)

var CONF_DIR = path.Join("/etc", "oakestra")
var CONF_FILE = path.Join(CONF_DIR, "conf.json")
var DEFAULT_LOG_DIR = "/tmp"
var DEFAULT_CNI = "NetManager"

func Execute() error {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	return rootCmd.Execute()
}

func init() {
	rootCmd.Flags().StringVarP(&clusterAddress, "clusterAddr", "a", "localhost", "Address of the cluster orchestrator without port")
	rootCmd.Flags().IntVarP(&clusterPort, "clusterPort", "p", 10100, "Port of the cluster orchestrator")
	rootCmd.Flags().IntVarP(&overlayNetwork, "netmanagerPort", "n", 0, "Port of the NetManager component, if configured. Otherwise the netmanager will look for a local socket. If no local socket the overlay network is disabled by default.")
	rootCmd.Flags().BoolVarP(&unikernelSupport, "unikernel", "u", false, "Enable Unikernel support. [qemu/kvm required]")
	rootCmd.Flags().StringVarP(&logDirectory, "logs", "l", DEFAULT_LOG_DIR, "Directory for application's logs")
}

func nodeEngineDaemonManager() error {

	if _, err := os.Stat(CONF_FILE); err != nil {
		// read cluster configuration if not present or new value set
		defaultConfig()
	}

	if clusterAddress != "localhost" {
		// read cluster configuration if not present or new value set
		configCluster(clusterAddress)
	}

	if logDirectory != DEFAULT_LOG_DIR {
		// read cluster configuration if not present or new value set
		configLogs(logDirectory)
	}

	if unikernelSupport {
		// read cluster configuration if not present or new value set
		setUnikernel(true)
	}

	if overlayNetwork > 0 {
		setNetworkPort(overlayNetwork)
	}

	// try to start the netmanager service if present
	cmd := exec.Command("systemctl", "start", "netmanager")
	_ = cmd.Run()

	// start the node engine daemon systemctl daemon
	cmd = exec.Command("systemctl", "start", "nodeengine")
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Println("NodeEngine started  🟢")

	return nil
}
