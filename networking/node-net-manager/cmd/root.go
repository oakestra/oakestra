package cmd

import (
	"NetManager/logger"
	"NetManager/network"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"NetManager/server"

	"github.com/spf13/cobra"
	"github.com/tkanos/gonfig"
)

var (
	rootCmd = &cobra.Command{
		Use:   "NetManager",
		Short: "Start a NetManager",
		Long:  `Start a New Oakestra Worker Node`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return startNetManager()
		},
	}
	cfgFile   string
	localPort int
)

const MONITORING_CYCLE = time.Second * 2

func Execute() error {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	return rootCmd.Execute()
}

func init() {
	rootCmd.Flags().StringVarP(&cfgFile, "cfg", "c", "/etc/netmanager/netcfg.json", "Path of the netcfg.json configuration file")
	rootCmd.Flags().IntVarP(&localPort, "port", "p", 0, "Set a custom port to expose the NetManager API, default is 0 (unix socket /etc/netmanager/netmanager.sock)")
}

func startNetManager() error {

	if !isAlreadyRunning() {

		err := gonfig.GetConf(cfgFile, &server.Configuration)
		if err != nil {
			log.Fatal(err)
		}

		if server.Configuration.Debug {
			logger.SetDebugMode()
		}

		log.Print(server.Configuration)

		network.IptableFlushAll()

		log.Println("NetManager started, but waiting for NodeEngine registration 🟠")
		server.HandleRequests(localPort)

		return nil
	} else {
		log.Println("NetManager already running, exiting")
	}
	return nil
}

func isAlreadyRunning() bool {
	procPath := strings.Split(os.Args[0], "/")
	currentProcName := procPath[len(procPath)-1]
	fmt.Printf("Checking for process: %s\n", currentProcName)
	cmd := exec.Command("pgrep", "-f", currentProcName)

	// Execute the command and capture the output
	out, err := cmd.Output()

	if err != nil {
		// Check for errors during command execution
		if err.(*exec.ExitError).ExitCode() == 1 {
			return false
		} else {
			fmt.Printf("Error checking for NetManager: %v\n", err)
		}
	} else {
		// Check output for processes with the same name (ignoring case)
		processes := strings.Split(string(out), "\n")
		// If more than 3 processes are found, it means that the current process is already running (3 bcause empty line+current process+pgrep process)
		if len(processes) > 3 {
			return true
		}
	}
	return false
}
