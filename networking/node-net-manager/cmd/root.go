package cmd

import (
	"NetManager/logger"
	"NetManager/model"
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
	cfgFile      string
	defaultIface string
	localPort    int
)

const MONITORING_CYCLE = time.Second * 2

func Execute() error {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	return rootCmd.Execute()
}

func init() {
	cfgFile = "/etc/netmanager/netcfg.json"
	rootCmd.Flags().StringVarP(&defaultIface, "defaultInterface", "i", "", "Sets the default interface (only relevant if the system has more than one)")
}

func startNetManager() error {

	err := gonfig.GetConf(cfgFile, &model.NetConfig)
	if err != nil {
		log.Fatal(err)
	}

	if model.NetConfig.Debug {
		logger.SetDebugMode()
	}

	model.NetConfig.DefaultInterface = defaultIface

	log.Print(model.NetConfig)

	network.IptableFlushAll()

	log.Println("NetManager started, but waiting for NodeEngine registration 🟠")
	server.HandleRequests(localPort)

	return nil

}
