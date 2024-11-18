package cmd

import (
	"bytes"
	"fmt"
	"go_node_engine/config"
	"os/exec"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

var (
	statusCmd = &cobra.Command{
		Use:   "status",
		Short: "check status of node engine",
		RunE: func(cmd *cobra.Command, args []string) error {
			return statusNodeEngine()
		},
	}
)

func statusNodeEngine() error {
	configManager := config.GetConfFileManager()
	confFile, err := configManager.Get()
	if err != nil {
		return err
	}

	// Define the command and arguments
	execCommandWithOutput("systemctl", "status", "nodeengine", "--no-pager")

	if confFile.OverlayNetwork == config.DEFAULT_CNI {
		//show net status if default cni active
		execCommandWithOutput("NetManager", "status")
	}

	return nil
}

func execCommandWithOutput(command string, args ...string) {
	cmd := exec.Command(command, args...)

	// Create pipes for capturing output streams
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	_ = cmd.Run()

	if stderr.Len() > 0 {
		fmt.Println(stderr.String())
	}
	fmt.Println(stdout.String())
}
