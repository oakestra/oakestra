package env

import (
	"errors"
	"fmt"
	"github.com/coreos/go-iptables/iptables"
	"log"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

var iptable, ipterr = iptables.New()
var chain = "OAKESTRA"

func IptableFlushAll() {
	_ = iptable.ClearChain("nat", chain)
	_ = iptable.DeleteChain("nat", chain)
	_ = iptable.Delete("nat", "PREROUTING", "-j", chain)
}

func disableReversePathFiltering(bridgeName string) {
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

func enableForwarding(bridgeName string, proxyName string) {
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
	_ = iptable.ClearChain("nat", chain)
	_ = iptable.DeleteChain("nat", chain)
	_ = iptable.NewChain("nat", chain)
	err = iptable.AppendUnique("nat", "PREROUTING", "-j", chain)
	if err != nil {
		log.Fatal(err.Error())
	}
}

func enableMasquerading(address string, mask string, bridgeName string, internetIfce string) {
	log.Println("add NAT ip MASQUERADING")
	err := iptable.AppendUnique("nat", "POSTROUTING", "-s", address+mask, "-o", bridgeName, "-j", "MASQUERADE")
	if err != nil {
		log.Fatal(err.Error())
	}
	log.Println("add NAT ip MASQUERADING for the bridge")
	err = iptable.AppendUnique("nat", "POSTROUTING", "-s", address+mask, "-o", internetIfce, "-j", "MASQUERADE")
	if err != nil {
		log.Fatal(err.Error())
	}
}

//oper or close container port with the nat rules
func manageContainerPorts(localContainerAddress string, portmapping string, operation PortOperation) error {
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
