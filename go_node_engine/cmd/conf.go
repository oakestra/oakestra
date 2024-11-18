package cmd

import (
	"fmt"
	"go_node_engine/config"
	"go_node_engine/logger"
	"go_node_engine/model"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(addClusterCmd)
	configCmd.AddCommand(logsConfCommand)
	configCmd.AddCommand(setVirtualizationCmd)
	configCmd.AddCommand(defaultConfigCmd)
	configCmd.AddCommand(setCni)
	configCmd.AddCommand(setAuth)
	setVirtualizationCmd.AddCommand(enableUnikernel)
	setCni.AddCommand(enableNetwork)
	setCni.AddCommand(disableNetwork)
	addClusterCmd.Flags().IntVarP(&clusterPort, "clusterPort", "p", 10100, "Custom port of the cluster orchestrator")
	setAuth.Flags().StringVarP(&certFile, "certFile", "c", "", "Path to certificate for TLS support")
	setAuth.Flags().StringVarP(&keyFile, "keyFile", "k", "", "Path to key for TLS support")
}

var (
	configCmd = &cobra.Command{
		Use:   "config",
		Short: "configure the node engine",
	}
	defaultConfigCmd = &cobra.Command{
		Use:   "default",
		Short: "generates the default configuration file",
		Run: func(cmd *cobra.Command, args []string) {
			defaultConfig()
		},
	}
	addClusterCmd = &cobra.Command{
		Use:   "cluster [url]",
		Short: "set the cluster address (and port)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			configCluster(args[0])
		},
	}
	logsConfCommand = &cobra.Command{
		Use:   "applogs [path]",
		Short: "Configure the log directory for the applications",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			configLogs(args[0])
		},
	}

	// --- VIRTUALIZATION SUPPORT
	setVirtualizationCmd = &cobra.Command{
		Use:   "virtualization",
		Short: "Enable/Disable a virtualization runtime support",
		Run: func(cmd *cobra.Command, args []string) {
			showVirtualization()
		},
	}
	enableUnikernel = &cobra.Command{
		Use:   "unikernel [on/off]",
		Short: "Enable/Disable unikernel support",
		Run: func(cmd *cobra.Command, args []string) {
			setUnikernel(args[0])
		},
	}

	// --- ADDONS
	setAddonCmd = &cobra.Command{
		Use:   "addon",
		Short: "Enable/Disable addons",
		Run: func(cmd *cobra.Command, args []string) {
			showAddons()
		},
	}
	enableBuilder = &cobra.Command{
		Use:   "imageBuilder [on/off]",
		Short: "Enable/Disable imageBuilder support",
		Long:  "Checks if the host has QEMU (apt's qemu-user-static) installed for building multi-platform images.",
		Run: func(cmd *cobra.Command, args []string) {
			setBuilder(args[0])
		},
	}
	enableFlops = &cobra.Command{
		Use:   "FLOps [on/off]",
		Short: "Enable/Disable FLOps support",
		Long:  "Enables the ML-data-server sidecar for data collection for FLOps learners.",
		Run: func(cmd *cobra.Command, args []string) {
			setFLOps(args[0])
		},
	}

	// --- NETOWORKING
	setCni = &cobra.Command{
		Use:   "network [on/off]",
		Short: "Enable/Disable networking support",
	}
	enableNetwork = &cobra.Command{
		Use:   "on",
		Short: "Enable overlay network support (Requires NetManager daemon running)",
		Run: func(cmd *cobra.Command, args []string) {
			setNetwork(config.DEFAULT_CNI)
		},
	}
	disableNetwork = &cobra.Command{
		Use:   "off",
		Short: "Disable overlay network support",
		Run: func(cmd *cobra.Command, args []string) {
			setNetwork("")
		},
	}

	// ---MQTT AUTH
	setAuth = &cobra.Command{
		Use:   "auth",
		Short: "Set Mqtt Authentication",
		Run: func(cmd *cobra.Command, args []string) {
			setMqttAuth()
		},
	}
)

func defaultConfig() error {
	configManager := config.GetConfFileManager()
	clusterConf := config.GenDefaultConfig()
	return configManager.Write(clusterConf)
}

func configCluster(address string) error {
	configManager := config.GetConfFileManager()
	clusterConf, err := configManager.Get()
	if err != nil {
		return err
	}

	clusterConf.ClusterAddress = address
	clusterConf.ClusterPort = clusterPort

	return configManager.Write(clusterConf)
}

func configLogs(path string) error {
	configManager := config.GetConfFileManager()
	clusterConf, err := configManager.Get()
	if err != nil {
		return err
	}

	clusterConf.AppLogs = path

	return configManager.Write(clusterConf)
}

func showVirtualization() error {

	configManager := config.GetConfFileManager()
	clusterConf, err := configManager.Get()
	if err != nil {
		return err
	}

	virts := []string{}

	for _, virt := range clusterConf.Virtualizations {
		status := "❌ Disabled"
		if virt.Active {
			status = "🟢 Active"
		}
		virts = append(virts, fmt.Sprintf("%s: %s", virt.Name, status))
	}

	fmt.Printf("Virtualization Runtimes:")
	for _, v := range virts {
		fmt.Println(v)
	}
	if len(virts) == 0 {
		fmt.Println("No Virtualization Runtime Configured.")
	}

	return nil
}

func setUnikernel(trigger string) error {
	active := false
	if trigger == "on" || trigger == "enable" || trigger == "true" {
		active = true
	}

	configManager := config.GetConfFileManager()
	clusterConf, err := configManager.Get()
	if err != nil {
		return err
	}

	UnikernelVirt := config.Virtualization{
		Name:    "unikraft",
		Runtime: string(model.UNIKERNEL_RUNTIME),
		Active:  active,
		Config:  []string{},
	}

	clusterConf.Virtualizations = append(clusterConf.Virtualizations, UnikernelVirt)

	return configManager.Write(clusterConf)
}

func showAddons() error {

	configManager := config.GetConfFileManager()
	clusterConf, err := configManager.Get()
	if err != nil {
		return err
	}

	addons := []string{}

	for _, add := range clusterConf.Addons {
		status := "❌ Disabled"
		if add.Active {
			status = "🟢 Active"
		}
		addons = append(addons, fmt.Sprintf("%s: %s", add.Name, status))
	}

	fmt.Printf("Configured Addons:")
	for _, v := range addons {
		fmt.Println(v)
	}
	if len(addons) == 0 {
		fmt.Println("No Addons Configured.")
	}

	return nil
}

func setBuilder(trigger string) error {
	cmd := exec.Command("dpkg", "-s", "qemu-user-static")
	output, err := cmd.Output()
	if err != nil || !strings.Contains(string(output), "ok installed") {
		logger.ErrorLogger().Fatalf("Unable to find qemu-user-static apt package for multi-platform image-builder: %v\n", err)
	}

	active := false
	if trigger == "on" || trigger == "enable" || trigger == "true" {
		active = true
	}

	configManager := config.GetConfFileManager()
	clusterConf, err := configManager.Get()
	if err != nil {
		return err
	}

	BuilderAddon := config.Addon{
		Name:   string(model.IMAGE_BUILDER),
		Active: active,
		Config: []string{},
	}

	clusterConf.Addons = append(clusterConf.Addons, BuilderAddon)

	return configManager.Write(clusterConf)
}

func setFLOps(trigger string) error {
	active := false
	if trigger == "on" || trigger == "enable" || trigger == "true" {
		active = true
	}

	configManager := config.GetConfFileManager()
	clusterConf, err := configManager.Get()
	if err != nil {
		return err
	}

	BuilderAddon := config.Addon{
		Name:   string(model.FLOPS_LEARNER),
		Active: active,
		Config: []string{},
	}

	clusterConf.Addons = append(clusterConf.Addons, BuilderAddon)

	return configManager.Write(clusterConf)
}

func setNetwork(cniName string) error {
	configManager := config.GetConfFileManager()
	clusterConf, err := configManager.Get()
	if err != nil {
		return err
	}

	clusterConf.OverlayNetwork = cniName

	return configManager.Write(clusterConf)
}

func setNetworkPort(netPort int) error {
	configManager := config.GetConfFileManager()
	clusterConf, err := configManager.Get()
	if err != nil {
		return err
	}

	clusterConf.NetPort = netPort

	return configManager.Write(clusterConf)
}

func setMqttAuth() error {

	configManager := config.GetConfFileManager()
	clusterConf, err := configManager.Get()
	if err != nil {
		return err
	}

	if certFile != "" {
		clusterConf.CertFile = certFile
	}
	if keyFile != "" {
		clusterConf.KeyFile = keyFile
	}

	return configManager.Write(clusterConf)
}
