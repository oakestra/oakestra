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
	// Define the command and arguments
	cmd := exec.Command("systemctl", "status", "nodeengine", "--no-pager")

	// Create pipes for capturing output streams
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	_ = cmd.Run()

	if stderr.Len() > 0 {
		fmt.Println(stderr.String())
	}
	fmt.Println(stdout.String())

	return nil
}
