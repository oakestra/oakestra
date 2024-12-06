package cmd

import (
	"NetManager/logger"
	"NetManager/network"
	"log"
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
	cfgFile = "/etc/netmanager/netcfg.json"
}

func startNetManager() error {

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

}
