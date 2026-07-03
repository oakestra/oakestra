package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"go_node_engine/logger"
	"os"
	"strings"
)

const (
	DEFAULT_LOG_DIR  = "/tmp"
	AUTO_OAK_NETWORK = "default"
	PUBLIC_IP_FALSE  = PublicIPMode("false")
	PUBLIC_IP_AUTO   = PublicIPMode("auto")

	confDir  = "/etc/oakestra"
	confPath = "/etc/oakestra/conf.json"
)

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

func (m PublicIPMode) normalized() string {
	return strings.TrimSpace(strings.ToLower(string(m)))
}

func (m PublicIPMode) IsDisabled() bool {
	normalized := m.normalized()
	return normalized == "" || normalized == string(PUBLIC_IP_FALSE)
}

func (m PublicIPMode) IsAuto() bool {
	normalized := m.normalized()
	return normalized == string(PUBLIC_IP_AUTO) || normalized == "true"
}

func (m PublicIPMode) Value() string {
	normalized := m.normalized()
	if normalized == "" || normalized == string(PUBLIC_IP_FALSE) {
		return string(PUBLIC_IP_FALSE)
	}
	if normalized == string(PUBLIC_IP_AUTO) || normalized == "true" {
		return string(PUBLIC_IP_AUTO)
	}
	return strings.TrimSpace(string(m))
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

	return fmt.Errorf("invalid public_ip mode: expected boolean or string, got %s", data)
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

func (c *ConfFile) Get() (ConfFile, error) {
	data, err := os.ReadFile(confPath)
	if errors.Is(err, os.ErrNotExist) || (err == nil && len(data) == 0) {
		logger.InfoLogger().Printf("Config file missing or empty, using default configuration")
		def := GenDefaultConfig()
		return def, c.Write(def)
	}
	if err != nil {
		return *c, err
	}

	var clusterConf ConfFile
	if err := json.Unmarshal(data, &clusterConf); err != nil {
		logger.ErrorLogger().Printf("Error reading configuration: %v, resetting the file\n", err)
		if resetErr := c.Write(GenDefaultConfig()); resetErr != nil {
			return *c, resetErr
		}
		return *c, err
	}
	return clusterConf, nil
}

func (c *ConfFile) Write(new ConfFile) error {
	data, err := json.Marshal(new)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(confDir, 0755); err != nil {
		logger.ErrorLogger().Printf("Failed to create config directory %s: %v\n", confDir, err)
		return err
	}
	return os.WriteFile(confPath, data, 0644)
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
