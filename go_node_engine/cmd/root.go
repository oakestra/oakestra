package cmd

import (
	"fmt"
	"go_node_engine/logger"
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
	clusterAddress string
	clusterPort    int
	detatched      bool
	// Addons
	imageBuilder        bool
	flopsLearnerSupport bool
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
}

func nodeEngineDaemonManager() error {
	if _, err := os.Stat(CONF_FILE); err != nil {
		err := defaultConfig()
		if err != nil {
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

	if certFile != "" || keyFile != "" {
		// set Mqtt auth parameters
		err := setMqttAuth()
		if err != nil {
			return err
		}
	}

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
	logFile, err := os.Open("/var/log/oakestra/nodeengine.log")
	if err != nil {
		fmt.Println("Error opening log file, is the NodeEngine running? Use 'NodeEngine status' to check.")
		return err
	}
	defer func() {
		err := logFile.Close()
		if err != nil {
			logger.ErrorLogger().Printf("Unable to close logfile")
		}
	}()

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
