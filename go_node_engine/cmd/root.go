package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"syscall"

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
	netmanagerPort   int
	overlayNetwork   bool
	unikernelSupport bool
	detatched        bool
	logDirectory     string
)

var CONF_DIR = path.Join("/etc", "oakestra")
var CONF_FILE = path.Join(CONF_DIR, "conf.json")
var DEFAULT_LOG_DIR = "/tmp"
var DEFAULT_CNI = "NetManager"
var DISABLE_NETWORK = "NoNetwork"


// Execute is the entry point of the NodeEngine
func Execute() error {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	return rootCmd.Execute()
}

func init() {
	rootCmd.Flags().StringVarP(&clusterAddress, "clusterAddr", "a", "localhost", "Address of the cluster orchestrator without port")
	rootCmd.Flags().IntVarP(&clusterPort, "clusterPort", "p", 10100, "Port of the cluster orchestrator")
	rootCmd.Flags().IntVarP(&netmanagerPort, "netmanagerPort", "n", 0, "Port of the NetManager component (deprecated).")
	rootCmd.Flags().BoolVarP(&overlayNetwork, "overlayNetwork", "o", true, "Enable or Disable local overlay network")
	rootCmd.Flags().BoolVarP(&unikernelSupport, "unikernel", "u", false, "Enable Unikernel support. [qemu/kvm required]")
	rootCmd.Flags().StringVarP(&logDirectory, "logs", "l", DEFAULT_LOG_DIR, "Directory for application's logs")
	rootCmd.Flags().BoolVarP(&detatched, "detatch", "d", false, "Run the NodeEngine in the background (daemon mode)")
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

	if overlayNetwork {
		setNetwork(DEFAULT_CNI)
	} else {
		setNetwork(DISABLE_NETWORK)
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
	if !detatched {
		return attatch()
	}

	return nil
}

func attatch() error {
	// Open the log file
	logFile, err := os.Open("/var/log/oakestra/nodeengine.log")
	if err != nil {
		fmt.Println("Error opening log file, is the NodeEngine running? Use 'NodeEngine status' to check.")
		return err
	}
	defer logFile.Close()

	// Get the file size to start reading from the end
	fileInfo, err := logFile.Stat()
	if err != nil {
		return err
	}

	// Track the current position in the file
	offset := fileInfo.Size()

	// Handle interrupt signal (Ctrl+C)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\nExiting...")
		stopNodeEngine()
		os.Exit(0)
	}()

	// Continuously tail the log file
	for {
		// Seek to the end of the file
		_, err = logFile.Seek(offset, io.SeekStart)
		if err != nil {
			return err
		}

		// Read new content from the file
		data, err := io.ReadAll(logFile)
		if err != nil && err != io.EOF {
			return err
		}

		fmt.Print(string(data))

		// Update the offset for the next read
		offset += int64(len(data))
	}
}
