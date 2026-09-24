package config

import (
	"crypto/rand"
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
	Mode            NodeMode         `json:"mode"`
	NodeUUID        string           `json:"node_uuid"`
	ClusterAddress  string           `json:"cluster_address"`
	ClusterSSL      bool             `json:"cluster_ssl"`
	ClusterPort     int              `json:"cluster_port"`
	AppLogs         string           `json:"app_logs"`
	OverlayNetwork  string           `json:"overlay_network"`
	PublicIp        PublicIPMode     `json:"public_ip"`
	NetPort         int              `json:"overlay_network_port"`
	CertFile        string           `json:"mqtt_cert_file"`
	KeyFile         string           `json:"mqtt_key_file"`
	P2P             P2PConfig        `json:"p2p"`
	Addons          []Addon          `json:"addons"`
	Virtualizations []Virtualization `json:"virtualizations"`
	CSIDrivers      []CSIDriverType  `json:"csi_drivers"`
}

// NodeMode selects whether the worker attaches to a cluster orchestrator or runs
// in the decentralised peer-to-peer mesh.
type NodeMode string

const (
	MODE_CLUSTER NodeMode = "cluster"
	MODE_P2P     NodeMode = "p2p"
)

// ParseNodeMode normalises a user-provided mode string, defaulting to cluster mode.
func ParseNodeMode(mode string) NodeMode {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case string(MODE_P2P):
		return MODE_P2P
	default:
		return MODE_CLUSTER
	}
}

// P2PConfig holds all peer-to-peer mode parameters.
// The gossip PSK keyring and the data-plane AEAD key are obtained at enrollment
// and stored under /etc/oakestra/p2p/ — they are NOT kept in this JSON.
type P2PConfig struct {
	BootstrapPeers   []string      `json:"bootstrap_peers"`          // optional static seeds "ip:port"
	Discovery        string        `json:"discovery"`                // "mdns" (default) | "static" | "both"
	NodeCert         string        `json:"node_cert"`                // this node's self-signed cert
	NodeKey          string        `json:"node_key"`                 // this node's private key (never leaves node)
	GossipPort       int           `json:"gossip_port"`              // default 50104
	EnrollPort       int           `json:"enroll_port"`              // TLS enrollment endpoint, default 50106
	ServiceIPBlock   string        `json:"service_ip_block"`         // claimed /24 in 10.30.0.0/16
	SchedSigma       int           `json:"sched_sigma"`              // quorum divisor, default 4
	SchedTimeoutSec  int           `json:"sched_timeout_sec"`        // bid-round timeout, default 30
	SchedRetryMaxSec int           `json:"sched_retry_max_sec"`      // PENDING retry backoff cap, default 300
	ReplicaFactor    int           `json:"replica_factor"`           // SLA custodians per job, default 3
	Pending          PendingEnroll `json:"pending_enroll,omitempty"` // one-shot join params (cleared after enrollment)
}

// PendingEnroll carries one-shot enrollment parameters from the boot/config command
// to the daemon, which runs the enrollment handshake and clears them.
type PendingEnroll struct {
	Init   bool   `json:"init"`    // genesis: create a new network
	Join   string `json:"join"`    // enrollment bootstrap address ip:port
	Token  string `json:"token"`   // single-use join token
	CaHash string `json:"ca_hash"` // trust-anchor pin (sha256:...)
}

// HasPendingEnroll reports whether the node still needs to run enrollment.
func (p P2PConfig) HasPendingEnroll() bool {
	return p.Pending.Init || p.Pending.Join != ""
}

// GenDefaultP2PConfig returns the P2P defaults. These are inert while Mode=cluster.
func GenDefaultP2PConfig() P2PConfig {
	return P2PConfig{
		BootstrapPeers:   []string{},
		Discovery:        "mdns",
		GossipPort:       50104,
		EnrollPort:       50106,
		SchedSigma:       4,
		SchedTimeoutSec:  30,
		SchedRetryMaxSec: 300,
		ReplicaFactor:    3,
	}
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
		Mode:           MODE_CLUSTER,
		ClusterAddress: "0.0.0.0",
		ClusterPort:    10100,
		ClusterSSL:     false,
		AppLogs:        DEFAULT_LOG_DIR,
		OverlayNetwork: AUTO_OAK_NETWORK,
		PublicIp:       PUBLIC_IP_FALSE,
		NetPort:        0,
		P2P:            GenDefaultP2PConfig(),
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

// GenerateNodeUUID returns a random RFC-4122-ish v4 UUID string. Used to give a
// p2p node a stable identity in place of the cluster-assigned id.
func GenerateNodeUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// Mode returns the configured node mode, defaulting to cluster if unset (back-compat
// with configs written before p2p mode existed).
func (c ConfFile) NodeModeOrDefault() NodeMode {
	if c.Mode == "" {
		return MODE_CLUSTER
	}
	return c.Mode
}
