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
		Short: "check status of Net Manager",
		Run: func(cmd *cobra.Command, args []string) {
			statusNetManager()
		},
	}
)

func statusNetManager() error {
	execCommandWithOutput("systemctl", "status", "netmanager", "--no-pager")
	execCommandWithOutput("bash", "-c", "cat /var/log/oakestra/netmanager.log | grep STARTUP_CONFIG | tail -n 1")
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
