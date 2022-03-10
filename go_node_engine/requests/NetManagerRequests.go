package requests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go_node_engine/model"
	"net/http"
)

type registerRequest struct {
	ClientId string `json:"client_id"`
}

type connectNetworkRequest struct {
	Pid            int    `json:"pid"`
	Appname        string `json:"appName"`
	Instancenumber int    `json:"instanceNumber"`
}

func AttachNetworkToTask(pid int, appname string, instance int) error {
	request := connectNetworkRequest{
		Pid:            pid,
		Appname:        appname,
		Instancenumber: instance,
	}
	jsonReq, err := json.Marshal(request)
	if err != nil {
		return err
	}

	response, err := http.Post(
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

func DetachNetworkFromTask(appname string, instance int) error {
	request := connectNetworkRequest{
		Pid:            -1,
		Appname:        appname,
		Instancenumber: instance,
	}
	jsonReq, err := json.Marshal(request)
	if err != nil {
		return err
	}

	response, err := http.Post(
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

func RegisterSelfToNetworkComponent() error {
	request := registerRequest{
		ClientId: model.GetNodeInfo().Id,
	}
	jsonReq, err := json.Marshal(request)
	if err != nil {
		return err
	}

	response, err := http.Post(
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
