package config

import (
	"encoding/json"
	"fmt"
	"go_node_engine/logger"
	"os"
	"strings"
)

var DEFAULT_LOG_DIR = "/tmp"
var AUTO_OAK_NETWORK = "default"
var PUBLIC_IP_FALSE = PublicIPMode("false")
var PUBLIC_IP_AUTO = PublicIPMode("auto")

// RuntimeType is the type of runtime that the node executes
type RuntimeType string

// RuntimeType constants
const (
	CONTAINER_RUNTIME RuntimeType = "docker"
	UNIKERNEL_RUNTIME RuntimeType = "unikernel"
	CROSVM_RUNTIME    RuntimeType = "crosvm"
)

type ConfFile struct {
	ConfVersion     string           `json:"conf_version"`
	ClusterAddress  string           `json:"cluster_address"`
	ClusterSSL      bool             `json:"cluster_ssl"`
	ClusterPort     int              `json:"cluster_port"`
	AppLogs         string           `json:"app_logs"`
	OverlayNetwork  string           `json:"overlay_network"`
	PublicIp        PublicIPMode     `json:"public_ip"`
	NetPort         int              `json:"overlay_network_port"`
	CertFile        string           `json:"mqtt_cert_file"`
	KeyFile         string           `json:"mqtt_key_file"`
	Addons          []Addon          `json:"addons"`
	Virtualizations []Virtualization `json:"virtualizations"`
	CSIDrivers      []CSIDriverType  `json:"csi_drivers"`
}

type PublicIPMode string

func ParsePublicIPMode(mode string) PublicIPMode {
	normalized := strings.TrimSpace(strings.ToLower(mode))
	switch normalized {
	case "", string(PUBLIC_IP_FALSE):
		return PUBLIC_IP_FALSE
	case string(PUBLIC_IP_AUTO), "true":
		return PUBLIC_IP_AUTO
	default:
		return PublicIPMode(strings.TrimSpace(mode))
	}
}

func (m PublicIPMode) IsDisabled() bool {
	return ParsePublicIPMode(string(m)) == PUBLIC_IP_FALSE
}

func (m PublicIPMode) IsAuto() bool {
	return ParsePublicIPMode(string(m)) == PUBLIC_IP_AUTO
}

func (m PublicIPMode) Value() string {
	return string(ParsePublicIPMode(string(m)))
}

func (m PublicIPMode) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.Value())
}

func (m *PublicIPMode) UnmarshalJSON(data []byte) error {
	var boolMode bool
	if err := json.Unmarshal(data, &boolMode); err == nil {
		if boolMode {
			*m = PUBLIC_IP_AUTO
			return nil
		}
		*m = PUBLIC_IP_FALSE
		return nil
	}

	var stringMode string
	if err := json.Unmarshal(data, &stringMode); err == nil {
		*m = ParsePublicIPMode(stringMode)
		return nil
	}

	return fmt.Errorf("invalid public_ip mode")
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

// CSIDriverType describes a locally available CSI plugin endpoint.
// The Endpoint must point to the plugin's UNIX domain socket, typically
// provided via the CSI_ENDPOINT environment variable or a per-plugin config.
type CSIDriverType struct {
	// Name is the CSI driver name returned by GetPluginInfo (e.g. "nfs.csi.k8s.io")
	Name string `json:"csi_driver_name"`
	// Endpoint is the UNIX domain socket path for this CSI plugin (e.g. "/var/lib/kubelet/plugins/nfs.csi.k8s.io/csi.sock")
	Endpoint string `json:"csi_driver_endpoint"`
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
		ClusterAddress: "0.0.0.0",
		ClusterPort:    10100,
		ClusterSSL:     false,
		AppLogs:        DEFAULT_LOG_DIR,
		OverlayNetwork: AUTO_OAK_NETWORK,
		PublicIp:       PUBLIC_IP_FALSE,
		NetPort:        0,
		Virtualizations: []Virtualization{
			{
				Name:    "containerd",
				Runtime: string(CONTAINER_RUNTIME),
				Active:  true,
				Config:  []string{},
			},
		},
	}
}
