package requests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go_node_engine/config"
	"go_node_engine/model"
	"go_node_engine/p2p"
	"net"
	"net/http"
	"sync"
	"time"
)

type registerRequest struct {
	ClientId       string `json:"client_id"`
	ClusterAddress string `json:"cluster_address"`
	// P2P mode fields. Mode "" or "cluster" => cluster mode.
	Mode        string   `json:"mode,omitempty"`
	GossipKey   string   `json:"gossip_key,omitempty"`
	Seeds       []string `json:"seeds,omitempty"`
	GossipPort  int      `json:"gossip_port,omitempty"`
	ControlPort int      `json:"control_port,omitempty"`
}

type connectNetworkRequest struct {
	Pid            int    `json:"pid"`
	Servicename    string `json:"serviceName"`
	Instancenumber int    `json:"instanceNumber"`
	PortMappings   string `json:"portMappings"`
}

type connectNetworkRequestUnikernel struct {
	Pid            int    `json:"pid"`
	Servicename    string `json:"serviceName"`
	Instancenumber int    `json:"instanceNumber"`
	PortMappings   string `json:"portMappings"`
}

var ongoingDeployment sync.Mutex

var httpClient = &http.Client{
	Timeout: time.Second * 10,
}

// AttachNetworkToTask attaches a network to a task
func AttachNetworkToTask(pid int, servicename string, instance int, portMappings string) error {

	ongoingDeployment.Lock()
	defer ongoingDeployment.Unlock()

	request := connectNetworkRequest{
		Pid:            pid,
		Servicename:    servicename,
		Instancenumber: instance,
		PortMappings:   portMappings,
	}
	jsonReq, err := json.Marshal(request)
	if err != nil {
		return err
	}

	response, err := httpClient.Post(
		fmt.Sprintf("http://localhost:%d/container/deploy", model.GetNodeInfo().NetManagerPort),
		"application/json",
		bytes.NewBuffer(jsonReq),
	)
	if err != nil {
		return err
	}
	if response.StatusCode != 200 {
		return fmt.Errorf("NetManager deploy failed, status code: %d", response.StatusCode)
	}
	return nil
}

// DetachNetworkFromTask detaches a network from a task
func DetachNetworkFromTask(servicename string, instance int) error {
	request := connectNetworkRequest{
		Pid:            -1,
		Servicename:    servicename,
		Instancenumber: instance,
	}
	jsonReq, err := json.Marshal(request)
	if err != nil {
		return err
	}

	response, err := httpClient.Post(
		fmt.Sprintf("http://localhost:%d/container/undeploy", model.GetNodeInfo().NetManagerPort),
		"application/json",
		bytes.NewBuffer(jsonReq),
	)
	if err != nil {
		return err
	}
	if response.StatusCode != 200 {
		return fmt.Errorf("NetManager undeploy failed, status code: %d", response.StatusCode)
	}
	return nil
}

// RegisterSelfToNetworkComponent registers the node to the network component
func RegisterSelfToNetworkComponent() error {
	request := registerRequest{
		ClientId:       model.GetNodeInfo().Id,
		ClusterAddress: model.GetNodeInfo().ClusterAddress,
	}

	// In p2p mode, hand the NetManager the gossip parameters instead of a cluster address
	// The NetManager then starts the p2p membership + registry resolver.
	if cfg, err := config.GetConfFileManager().Get(); err == nil && cfg.NodeModeOrDefault() == config.MODE_P2P {
		request.Mode = "p2p"
		request.ClientId = cfg.NodeUUID
		request.GossipPort = cfg.P2P.GossipPort
		request.ControlPort = p2p.DefaultControlPort
		seeds := cfg.P2P.BootstrapPeers
		// The gossip PSK and seed list come from enrollment/genesis.
		if creds, ok := p2p.LoadCreds(); ok {
			request.GossipKey = creds.GossipKey
			seeds = append(seeds, creds.Seeds...)
		}
		request.Seeds = seeds
	}

	jsonReq, err := json.Marshal(request)
	if err != nil {
		return err
	}

	if model.GetNodeInfo().NetManagerPort == 0 {
		// if not network port specified, attempt using local socket
		httpClient = &http.Client{
			Timeout: time.Second * 10,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", model.GetNodeInfo().OverlaySocket)
				},
			},
		}
	}

	response, err := httpClient.Post(
		fmt.Sprintf("http://localhost:%d/register", model.GetNodeInfo().NetManagerPort),
		"application/json",
		bytes.NewBuffer(jsonReq),
	)

	if err != nil {
		return err
	}
	if response.StatusCode != 200 {
		return fmt.Errorf("NetManager registration failed, status code: %d", response.StatusCode)
	}
	return nil
}

// CreateNetworkNamespaceForUnikernel creates a network namespace for a unikernel
func CreateNetworkNamespaceForUnikernel(servicename string, instance int, portMappings string) error {

	ongoingDeployment.Lock()
	defer ongoingDeployment.Unlock()

	request := connectNetworkRequestUnikernel{
		Pid:            0,
		Servicename:    servicename,
		Instancenumber: instance,
		PortMappings:   portMappings,
	}
	jsonReq, err := json.Marshal(request)
	if err != nil {
		return err
	}

	response, err := httpClient.Post(
		fmt.Sprintf("http://localhost:%d/unikernel/deploy", model.GetNodeInfo().NetManagerPort),
		"application/json",
		bytes.NewBuffer(jsonReq),
	)
	if err != nil {
		return err
	}
	if response.StatusCode != 200 {
		return fmt.Errorf("NetManager deploy failed, status code: %d", response.StatusCode)
	}
	return nil
}

// DeleteNamespaceForUnikernel deletes a network namespace for a unikernel
func DeleteNamespaceForUnikernel(servicename string, instance int) error {
	request := connectNetworkRequest{
		Pid:            -1,
		Servicename:    servicename,
		Instancenumber: instance,
	}
	jsonReq, err := json.Marshal(request)
	if err != nil {
		return err
	}

	response, err := httpClient.Post(
		fmt.Sprintf("http://localhost:%d/unikernel/undeploy", model.GetNodeInfo().NetManagerPort),
		"application/json",
		bytes.NewBuffer(jsonReq),
	)
	if err != nil {
		return err
	}
	if response.StatusCode != 200 {
		return fmt.Errorf("NetManager undeploy failed, status code: %d", response.StatusCode)
	}
	return nil
}
