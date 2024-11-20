package cmd

import (
	"bytes"
	"fmt"
	"go_node_engine/logger"
	"os/exec"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(stopCmd)
}

var (
	stopCmd = &cobra.Command{
		Use:   "stop",
		Short: "stops the NodeEngine (and NetManager if configured)",
		Run: func(cmd *cobra.Command, args []string) {
			stopNodeEngine()
		},
	}
)

func stopNodeEngine() {
	cmd := exec.Command("systemctl", "stop", "nodeengine", "--no-pager")

	// Create pipes for capturing output streams
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		logger.ErrorLogger().Printf("%v", err)
	}

	if stderr.Len() > 0 {
		fmt.Println(stderr.String())
	}
	fmt.Println(stdout.String())

	// Stop the NetManager service
	cmd = exec.Command("systemctl", "stop", "netmanager", "--no-pager")
	err = cmd.Run()
	if err != nil {
		logger.ErrorLogger().Printf("%v", err)
	}
}
