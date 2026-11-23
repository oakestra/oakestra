package model

type NetConfiguration struct {
	NodePublicAddress  string
	NodePublicPort     string
	ClusterUrl         string
	ClusterMqttPort    string
	DefaultInterface   string
	Debug              bool
	PublicIPNetworking bool
	MqttCert           string
	MqttKey            string
}

var NetConfig NetConfiguration
var WorkerID string

// P2PMode is set true when the NetManager is registered in peer-to-peer mode (no cluster
// MQTT control plane). Deployment status / interest / address-change updates then go to
// the gossip registry instead of MQTT — the MQTT client is never initialised, so those
// code paths must be skipped.
var P2PMode bool
