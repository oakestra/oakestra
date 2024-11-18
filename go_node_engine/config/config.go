package config

import (
	"encoding/json"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"os"
)

var DEFAULT_LOG_DIR = "/tmp"
var DEFAULT_CNI = "default"

type ConfFile struct {
	ConfVersion     string           `json:"conf_version"`
	ClusterAddress  string           `json:"cluster_address"`
	ClusterPort     int              `json:"cluster_port"`
	AppLogs         string           `json:"app_logs"`
	OverlayNetwork  string           `json:"overlay_network"`
	NetPort         int              `json:"overlay_network_port"`
	CertFile        string           `json:"mqtt_cert_file"`
	KeyFile         string           `json:"mqtt_key_file"`
	Addons          []Addon          `json:"addons"`
	Virtualizations []Virtualization `json:"virtualizations"`
}

type Addon struct {
	Name   string   `json:"addon_name"`
	Active bool     `json:"addon_active"`
	Config []string `json:"addon_config"`
}

type Virtualization struct {
	Name    string   `json:"virutalizaiton_name"`
	Runtime string   `json:"virutalizaiton_runtime"`
	Active  bool     `json:"virutalizaiton_active"`
	Config  []string `json:"virutalizaiton_config"`
}

type ConfFileManager interface {
	Get() (ConfFile, error)
	Write(ConfFile) error
}

func GetConfFileManager() ConfFileManager {
	f := ConfFile{}
	return &f
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
			err := confFile.Truncate(0)
			if err != nil {
				return nil, ConfFile{}, err
			}
			return nil, ConfFile{}, err

		}
	}

	return confFile, clusterConf, nil
}

func (c *ConfFile) Get() (ConfFile, error) {
	confFile, configF, err := getConfFile()
	if err != nil {
		return *c, err
	}
	defer func() {
		err := confFile.Close()
		if err != nil {
			logger.ErrorLogger().Printf("%v\n", err)
		}
	}()
	return configF, nil
}

func (c *ConfFile) Write(new ConfFile) error {
	c = &new

	marshalled, err := json.Marshal(c)
	if err != nil {
		fmt.Println(err)
		return err
	}

	confFile, _, err := getConfFile()
	if err != nil {
		return err
	}
	defer func() {
		err := confFile.Close()
		if err != nil {
			logger.ErrorLogger().Printf("%v\n", err)
		}
	}()

	err = confFile.Truncate(0)
	if err != nil {
		return err
	}
	_, err = confFile.Seek(0, 0)
	if err != nil {
		return err
	}
	_, err = confFile.Write(marshalled)
	if err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

func GenDefaultConfig() ConfFile {
	return ConfFile{
		ConfVersion:    "1.0",
		ClusterAddress: "localhost",
		ClusterPort:    10100,
		AppLogs:        DEFAULT_LOG_DIR,
		OverlayNetwork: DEFAULT_CNI,
		NetPort:        0,
		Virtualizations: []Virtualization{
			{
				Name:    "containerd",
				Runtime: string(model.CONTAINER_RUNTIME),
				Active:  true,
				Config:  []string{},
			},
		},
	}
}
