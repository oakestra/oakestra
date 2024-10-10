package cmd

import (
	"bytes"
	"fmt"
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
		Run: func(cmd *cobra.Command, args []string) {
			statusNodeEngine()
		},
	}
)

func statusNodeEngine() error {
	_, confFile, err := getConfFile()
	if err != nil {
		return err
	}

	// Define the command and arguments
	execCommandWithOutput("systemctl", "status", "nodeengine", "--no-pager")

	if confFile.OverlayNetwork == DEFAULT_CNI {
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
