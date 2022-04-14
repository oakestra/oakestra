package network

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type PortOperation string

const (
	OpenPorts  PortOperation = "-A"
	ClosePorts PortOperation = "-D"
)

var chain = "OAKESTRA"
var iptable = NewOakestraIpTable()

func IptableFlushAll() {
	_ = iptable.DeleteChain("nat", chain)
	_ = iptable.Delete("nat", "PREROUTING", "-j", chain)
}

func DisableReversePathFiltering(bridgeName string) {
	log.Println("disabling reverse path filtering")
	cmd := exec.Command("echo", "0", ">", "/proc/sys/net/ipv4/conf/all/rp_filter")
	err := cmd.Run()
	if err != nil {
		log.Fatal(err.Error())
	}
	log.Println("enabling IP forwarding")
	cmd = exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1")
	err = cmd.Run()
	if err != nil {
		log.Fatal(err.Error())
	}
	cmd = exec.Command("echo", "0", ">", "/proc/sys/net/ipv4/conf/"+bridgeName+"/rp_filter")
	err = cmd.Run()
	if err != nil {
		log.Fatal(err.Error())
	}
}

func EnableForwarding(bridgeName string, proxyName string) {
	log.Println("enabling tun device forwarding")
	err := iptable.AppendUnique("filter", "FORWARD", "-i", bridgeName, "-o", proxyName, "-j", "ACCEPT")
	err = iptable.AppendUnique("filter", "FORWARD", "-o", bridgeName, "-i", proxyName, "-j", "ACCEPT")
	if err != nil {
		log.Fatal(err.Error())
	}
	err = iptable.AppendUnique("filter", "FORWARD", "-o", bridgeName, "-j", "ACCEPT")
	if err != nil {
		log.Fatal(err.Error())
	}
	err = iptable.AppendUnique("filter", "FORWARD", "-i", bridgeName, "-j", "ACCEPT")
	if err != nil {
		log.Fatal(err.Error())
	}
	_ = iptable.DeleteChain("nat", chain)
	_ = iptable.AddChain("nat", chain)
	err = iptable.AppendUnique("nat", "PREROUTING", "-j", chain)
	if err != nil {
		log.Fatal(err.Error())
	}
}

func EnableMasquerading(address string, mask string, bridgeName string, internetIfce string) {

	log.Printf("add NAT ip MASQUERADING towards %s\n", internetIfce)
	err := iptable.AppendUnique("nat", "POSTROUTING", "-s", address+mask, "-o", internetIfce, "-j", "MASQUERADE")
	if err != nil {
		log.Fatal(err.Error())
	}

	//masquerating towards additional interfaces
	ifaces := []string{"en", "eth", "wl"}
	localifces, _ := net.Interfaces()
	for _, ifc := range localifces {
		for _, pattern := range ifaces {
			if ifc.Name != internetIfce && strings.Contains(ifc.Name, pattern) {
				log.Printf("add additional NAT ip MASQUERADING towards %s\n", ifc.Name)
				err := iptable.AppendUnique("nat", "POSTROUTING", "-s", address+mask, "-o", ifc.Name, "-j", "MASQUERADE")
				if err != nil {
					log.Fatal(err.Error())
				}
			}
		}
	}

}

// ManageContainerPorts open or close container port with the nat rules
func ManageContainerPorts(localContainerAddress string, portmapping string, operation PortOperation) error {
	if portmapping == "" {
		return nil
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	mappings := strings.Split(portmapping, ";")

	for _, portmap := range mappings {

		portType := "tcp"
		if strings.Contains(portmap, "/udp") {
			portmap = strings.Replace(portmap, "/udp", "", -1)
			portType = "udp"
		} else {
			portmap = strings.Replace(portmap, "/tcp", "", -1)
		}
		ports := strings.Split(portmap, ":")
		hostPort := ports[0]
		containerPort := ports[0]
		if len(ports) > 1 {
			containerPort = ports[1]
		}
		if !isValidPort(hostPort) || !isValidPort(containerPort) {
			return errors.New("invaid Port Mapping")
		}
		destination := fmt.Sprintf("%s:%s", localContainerAddress, containerPort)
		args := []string{"-p", portType, "--dport", hostPort, "-j", "DNAT", "--to-destination", destination}

		err := errors.New("invalid Operation")
		if operation == OpenPorts {
			err = iptable.Append("nat", chain, args...)
		}
		if operation == ClosePorts {
			err = iptable.Delete("nat", chain, args...)
		}
		if err != nil {
			log.Printf("ERROR: %v", err)
			return err
		}
		log.Printf("Changed port %s status toward destination %s\n", hostPort, destination)

	}
	return nil
}

//check if the string is a valid network port
func isValidPort(port string) bool {
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return false
	}
	if portInt < 0 || portInt > 65535 {
		return false
	}
	return true
}
