package model

type NetConfiguration struct {
	NodeAddress        string
	NodePort           string
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
