package cmd

import (
	"NetManager/logger"
	"NetManager/network"
	"flag"
	"log"
	"time"

	"NetManager/server"

	"github.com/kardianos/service"
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
	cfgFile    string
	localPort  int
	debug      bool
	daemonMode bool
)

type serviceDaemon struct{}

func (p *serviceDaemon) Start(s service.Service) error {
	go p.run()
	return nil
}

func (p *serviceDaemon) run() {
	for {
		time.Sleep(MONITORING_CYCLE)
		logger.InfoLogger().Println("NetManager is running 🟢")
	}
}

func (p *serviceDaemon) Stop(s service.Service) error {
	return nil
}

const MONITORING_CYCLE = time.Second * 2

func Execute() error {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	return rootCmd.Execute()
}

func init() {
	rootCmd.Flags().StringVarP(&cfgFile, "cfg", "c", "/etc/netmanager/netcfg.json", "Path of the netcfg.json configuration file")
	rootCmd.Flags().IntVarP(&localPort, "port", "p", 6000, "Default local port of the NetManager")
	rootCmd.Flags().BoolVarP(&debug, "debug", "D", false, "Enable debug logs")
	rootCmd.Flags().BoolVarP(&daemonMode, "detatch", "d", false, "Enable deatched mode (daemon mode))")
}

func startNetManager() error {
	cfgFile := flag.String("cfg", "/etc/netmanager/netcfg.json", "Set a cluster IP")
	localPort := flag.Int("p", 6000, "Default local port of the NetManager")
	debugMode := flag.Bool("D", false, "Debug mode, it enables debug-level logs")
	flag.Parse()

	err := gonfig.GetConf(*cfgFile, &server.Configuration)
	if err != nil {
		log.Fatal(err)
	}

	if *debugMode {
		logger.SetDebugMode()
	}

	log.Print(server.Configuration)

	network.IptableFlushAll()

	log.Println("NetManager started 🟢")

	if daemonMode {
		svcConfig := &service.Config{
			Name:        "OakestraNetManager",
			DisplayName: "Oakestra NetManager Daemon",
			Description: "Overlay network component of the Oakestra platform",
		}

		prg := &serviceDaemon{}
		s, err := service.New(prg, svcConfig)
		if err != nil {
			log.Fatal(err)
		}
		logger, err := s.Logger(nil)
		if err != nil {
			log.Fatal(err)
		}
		err = s.Run()
		if err != nil {
			logger.Error(err)
		}
	} else {
		server.HandleRequests(*localPort)
	}

	return nil
}
