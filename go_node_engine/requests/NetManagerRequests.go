package requests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go_node_engine/model"
	"net"
	"net/http"
	"sync"
	"time"
)

type registerRequest struct {
	ClientId       string `json:"client_id"`
	ClusterAddress string `json:"cluster_address"`
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
		return errors.New(fmt.Sprintf("NetManager deploy failed, status code: %d", response.StatusCode))
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
		return errors.New(fmt.Sprintf("NetManager undeploy failed, status code: %d", response.StatusCode))
	}
	return nil
}

// RegisterSelfToNetworkComponent registers the node to the network component
func RegisterSelfToNetworkComponent() error {
	request := registerRequest{
		ClientId:       model.GetNodeInfo().Id,
		ClusterAddress: model.GetNodeInfo().ClusterAddress,
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
		return errors.New(fmt.Sprintf("NetManager registration failed, status code: %d", response.StatusCode))
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
		return errors.New(fmt.Sprintf("NetManager deploy failed, status code: %d", response.StatusCode))
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
		return errors.New(fmt.Sprintf("NetManager undeploy failed, status code: %d", response.StatusCode))
	}
	return nil
}
