package model

type NetConfiguration struct {
	NodePublicAddress string
	NodePublicPort    string
	ClusterUrl        string
	ClusterMqttPort   string
	DefaultInterface  string
	Debug             bool
	MqttCert          string
	MqttKey           string
}

var NetConfig NetConfiguration
var WorkerID string
