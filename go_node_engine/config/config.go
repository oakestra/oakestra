package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"go_node_engine/logger"
	"os"
	"strings"
	"sync"
)

const (
	DefaultLogDir  = "/tmp"
	AutoOakNetwork = "default"
	PublicIPFalse  = PublicIPMode("false")
	PublicIPAuto   = PublicIPMode("auto")
)

// Need to use a variable for the config path so that tests can override it
var (
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
	case "", string(PublicIPFalse):
		return PublicIPFalse
	case string(PublicIPAuto), "true":
		return PublicIPAuto
	default:
		return PublicIPMode(strings.TrimSpace(mode))
	}
}

func (m PublicIPMode) normalized() string {
	return strings.TrimSpace(strings.ToLower(string(m)))
}

func (m PublicIPMode) IsDisabled() bool {
	normalized := m.normalized()
	return normalized == "" || normalized == string(PublicIPFalse)
}

func (m PublicIPMode) IsAuto() bool {
	normalized := m.normalized()
	return normalized == string(PublicIPAuto) || normalized == "true"
}

func (m PublicIPMode) Value() string {
	normalized := m.normalized()
	if normalized == "" || normalized == string(PublicIPFalse) {
		return string(PublicIPFalse)
	}
	if normalized == string(PublicIPAuto) || normalized == "true" {
		return string(PublicIPAuto)
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
			*m = PublicIPAuto
			return nil
		}
		*m = PublicIPFalse
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
	Name    string   `json:"virtualization_name"`
	Runtime string   `json:"virtualization_runtime"`
	Active  bool     `json:"virtualization_active"`
	Config  []string `json:"virtualization_config"`
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

// mu serializes all reads and writes to confPath within this process.
var mu sync.Mutex

// Read loads the node configuration from /etc/oakestra/conf.json. If the file
// is missing or empty it writes and returns the default configuration.
func Read() (ConfFile, error) {
	mu.Lock()
	defer mu.Unlock()

	data, err := os.ReadFile(confPath)
	if errors.Is(err, os.ErrNotExist) || (err == nil && len(data) == 0) {
		logger.InfoLogger().Printf("Config file missing or empty, using default configuration")
		def := Default()
		return def, writeLocked(def)
	}
	if err != nil {
		return ConfFile{}, err
	}

	var clusterConf ConfFile
	if err := json.Unmarshal(data, &clusterConf); err != nil {
		logger.ErrorLogger().Printf("Error reading configuration: %v, resetting the file\n", err)
		if resetErr := writeLocked(Default()); resetErr != nil {
			return ConfFile{}, resetErr
		}
		return ConfFile{}, err
	}
	return clusterConf, nil
}

// Write persists the given node configuration to /etc/oakestra/conf.json,
// overwriting any existing content.
func Write(conf ConfFile) error {
	mu.Lock()
	defer mu.Unlock()
	return writeLocked(conf)
}

// writeLocked assumes mu is already held by the caller.
func writeLocked(conf ConfFile) error {
	data, err := json.Marshal(conf)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(confDir, 0755); err != nil {
		logger.ErrorLogger().Printf("Failed to create config directory %s: %v\n", confDir, err)
		return err
	}
	return os.WriteFile(confPath, data, 0644)
}

// Default returns the built-in node configuration used when no config file
// exists yet or an existing one cannot be parsed.
func Default() ConfFile {
	return ConfFile{
		ConfVersion:    "1.0",
		ClusterAddress: "0.0.0.0",
		ClusterPort:    10100,
		ClusterSSL:     false,
		AppLogs:        DefaultLogDir,
		OverlayNetwork: AutoOakNetwork,
		PublicIp:       PublicIPFalse,
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
