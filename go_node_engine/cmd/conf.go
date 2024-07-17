package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type ConfFile struct {
	ClusterAddress   string `json:"cluster_address"`
	ClusterPort      int    `json:"cluster_port"`
	AppLogs          string `json:"app_logs"`
	UnikernelSupport bool   `json:"unikernel_support"`
	OverlayNetwork   string `json:"overlay_network"`
	NetPort          int    `json:"overlay_network_port"`
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(addClusterCmd)
	configCmd.AddCommand(logsConfCommand)
	configCmd.AddCommand(setUnikernelCmd)
	configCmd.AddCommand(defaultConfigCmd)
	configCmd.AddCommand(setCni)
	setUnikernelCmd.AddCommand(enableUnikernel)
	setUnikernelCmd.AddCommand(disableUnikernel)
	setCni.AddCommand(enableNetwork)
	setCni.AddCommand(disableNetwork)
	addClusterCmd.Flags().IntVarP(&clusterPort, "clusterPort", "p", 10100, "Custom port of the cluster orchestrator")
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
	setUnikernelCmd = &cobra.Command{
		Use:   "unikernel",
		Short: "Enable/Disable unikernel support",
	}
	enableUnikernel = &cobra.Command{
		Use:   "enable",
		Short: "Enable unikernel support",
		Run: func(cmd *cobra.Command, args []string) {
			setUnikernel(true)
		},
	}
	disableUnikernel = &cobra.Command{
		Use:   "disable",
		Short: "Disable unikernel support",
		Run: func(cmd *cobra.Command, args []string) {
			setUnikernel(false)
		},
	}
	setCni = &cobra.Command{
		Use:   "network",
		Short: "Enable/Disable networking support",
	}
	enableNetwork = &cobra.Command{
		Use:   "enable",
		Short: "Enable overlay network support (Requires NetManager daemon running)",
		Run: func(cmd *cobra.Command, args []string) {
			setNetwork(DEFAULT_CNI)
		},
	}
	disableNetwork = &cobra.Command{
		Use:   "disable",
		Short: "Disable overlay network support",
		Run: func(cmd *cobra.Command, args []string) {
			setNetwork("")
		},
	}
)

func defaultConfig() error {

	//create dir /etc/oakestra if not present
	err := os.MkdirAll("/etc/oakestra", 0755)
	if err != nil {
		fmt.Println(err)
		return err
	}

	//create file /etc/oakestra/cluster.cfg with the cluster address and port
	confFile, err := os.Create("/etc/oakestra/conf.json")
	if err != nil {
		fmt.Println(err)
		return err
	}

	clusterConf := ConfFile{
		ClusterAddress:   "localhost",
		ClusterPort:      10100,
		AppLogs:          DEFAULT_LOG_DIR,
		UnikernelSupport: false,
		OverlayNetwork:   DEFAULT_CNI,
		NetPort:          0,
	}

	marshalled, err := json.Marshal(clusterConf)
	if err != nil {
		fmt.Println(err)
		return err
	}

	//write cluster configuration
	confFile.Truncate(0)
	confFile.Seek(0, 0)
	_, err = confFile.Write(marshalled)
	if err != nil {
		fmt.Println(err)
		return err
	}

	return nil

}

func configCluster(address string) error {

	confFile, clusterConf, err := getConfFile()
	if err != nil {
		return err
	}
	defer confFile.Close()

	clusterConf.ClusterAddress = address
	clusterConf.ClusterPort = clusterPort

	marshalled, err := json.Marshal(clusterConf)
	if err != nil {
		fmt.Println(err)
		return err
	}

	//write cluster configuration
	confFile.Truncate(0)
	confFile.Seek(0, 0)
	_, err = confFile.Write(marshalled)
	if err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

func configLogs(path string) error {

	clusterConf := ConfFile{}

	confFile, err := os.Open("/etc/oakestra/conf.json")
	if err != nil {
		//create dir /etc/oakestra if not present
		err := os.MkdirAll("/etc/oakestra", 0755)
		if err != nil {
			fmt.Println(err)
			return err
		}

		//create file /etc/oakestra/cluster.cfg with the cluster address and port
		confFile, err = os.Create("/etc/oakestra/conf.json")
		if err != nil {
			fmt.Println(err)
			return err
		}
	} else {
		//read cluster configuration
		buffer := make([]byte, 2048)
		n, err := confFile.Read(buffer)
		if err != nil {
			return err
		}
		err = json.Unmarshal(buffer[:n], &clusterConf)
		if err != nil {
			fmt.Printf("Error reading configuration: %v\n, resetting the file", err)
		}
	}
	defer confFile.Close()

	clusterConf.AppLogs = path

	marshalled, err := json.Marshal(clusterConf)
	if err != nil {
		fmt.Println(err)
		return err
	}

	//write cluster configuration
	confFile.Truncate(0)
	confFile.Seek(0, 0)
	_, err = confFile.Write(marshalled)
	if err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

func setUnikernel(status bool) error {

	confFile, clusterConf, err := getConfFile()
	if err != nil {
		return err
	}
	defer confFile.Close()

	clusterConf.UnikernelSupport = status

	marshalled, err := json.Marshal(clusterConf)
	if err != nil {
		fmt.Println(err)
		return err
	}

	//write cluster configuration
	confFile.Truncate(0)
	confFile.Seek(0, 0)
	_, err = confFile.Write(marshalled)
	if err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

func setNetwork(cniName string) error {

	confFile, clusterConf, err := getConfFile()
	if err != nil {
		return err
	}
	defer confFile.Close()

	clusterConf.OverlayNetwork = cniName

	marshalled, err := json.Marshal(clusterConf)
	if err != nil {
		fmt.Println(err)
		return err
	}

	//write cluster configuration
	confFile.Truncate(0)
	confFile.Seek(0, 0)
	_, err = confFile.Write(marshalled)
	if err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

func setNetworkPort(netPort int) error {

	confFile, clusterConf, err := getConfFile()
	if err != nil {
		return err
	}
	defer confFile.Close()

	clusterConf.NetPort = netPort

	marshalled, err := json.Marshal(clusterConf)
	if err != nil {
		fmt.Println(err)
		return err
	}

	//write cluster configuration
	confFile.Truncate(0)
	confFile.Seek(0, 0)
	_, err = confFile.Write(marshalled)
	if err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

func getConfFile() (*os.File, ConfFile, error) {

	clusterConf := ConfFile{}

	confFile, err := os.OpenFile("/etc/oakestra/conf.json", os.O_RDWR, 0644)
	if err != nil {
		//create dir /etc/oakestra if not present
		err := os.MkdirAll("/etc/oakestra", 0755)
		if err != nil {
			fmt.Println(err)
			return nil, ConfFile{}, err
		}

		//create file /etc/oakestra/cluster.cfg with the cluster address and port
		confFile, err = os.Create("/etc/oakestra/conf.json")
		if err != nil {
			fmt.Println(err)
			return nil, ConfFile{}, err
		}
	} else {
		//read cluster configuration
		buffer := make([]byte, 2048)
		n, err := confFile.Read(buffer)
		if err != nil {
			return nil, ConfFile{}, err
		}
		err = json.Unmarshal(buffer[:n], &clusterConf)
		if err != nil {
			fmt.Printf("Error reading configuration: %v\n, resetting the file", err)
			confFile.Truncate(0)
			return nil, ConfFile{}, err

		}
	}

	return confFile, clusterConf, nil
}
