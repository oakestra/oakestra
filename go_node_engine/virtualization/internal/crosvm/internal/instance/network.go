package instance

import (
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/requests"
	"go_node_engine/util/taskid"
	"go_node_engine/virtualization/internal/network"
	"slices"
)

type networkConfig struct {
	Mac             string
	AddressIpv4Cidr string
	GatewayIpv4     string
}

func setupNetwork(service model.Service) (*networkConfig, error) {
	if !model.GetNodeInfo().Overlay {
		return nil, nil
	}

	if err := requests.CreateNetworkNamespaceForUnikernel(service.Sname, service.Instance, service.Ports); err != nil {
		logger.ErrorLogger().Printf("network creation failed: %v", err)
		return nil, err
	}

	mac, err := network.RetrieveTapMacInNamespace(taskid.GenerateForModel(&service))
	if err != nil {
		return nil, err
	}

	return &networkConfig{
		Mac:             mac,
		AddressIpv4Cidr: "192.168.1.2/30",
		GatewayIpv4:     "192.168.1.1",
	}, nil
}

func teardownNetwork(service model.Service) error {
	if !model.GetNodeInfo().Overlay {
		return nil
	}

	if err := requests.DeleteNamespaceForUnikernel(service.Sname, service.Instance); err != nil {
		logger.ErrorLogger().Printf("network deletion failed: %v", err)
		return err
	}

	return nil
}

func wrapCommandWithIpNetnsExec(service model.Service, executable string, args []string) (string, []string) {
	// the network namespace name is the task id of the instance
	return "ip", slices.Concat([]string{"netns", "exec", taskid.GenerateForModel(&service), executable}, args)
}
