package instance

import (
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/requests"
	"go_node_engine/util/taskid"
	"go_node_engine/virtualization/internal/network"
	"net"
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

	// the network namespace name is the task id of the instance
	addressIpv4, gatewayIpv4, addressIpv4Mask, mac, err := network.DeleteDefaultIpGwMask(taskid.GenerateForModel(&service))
	if err != nil {
		return nil, err
	}

	addressIpv4CidrSuffix, err := convertIpv4MaskToCIDRSuffix(addressIpv4Mask)
	if err != nil {
		return nil, err
	}

	addressIpv4Cidr := fmt.Sprintf("%s/%d", addressIpv4, addressIpv4CidrSuffix)

	return &networkConfig{
		Mac:             mac,
		AddressIpv4Cidr: addressIpv4Cidr,
		GatewayIpv4:     gatewayIpv4,
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

func convertIpv4MaskToCIDRSuffix(ipv4Mask string) (int, error) {
	ip := net.ParseIP(ipv4Mask)
	if ip == nil {
		return 0, fmt.Errorf("invalid IP address: %s", ipv4Mask)
	}

	ipv4 := ip.To4()
	if ipv4 == nil {
		return 0, fmt.Errorf("not an IPv4 address: %s", ipv4Mask)
	}

	ones, _ := net.IPMask(ipv4).Size()
	return ones, nil
}
