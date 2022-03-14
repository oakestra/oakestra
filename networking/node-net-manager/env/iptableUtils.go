package env

import (
	"log"
	"os/exec"
)

func iptableFlushAll() {
	log.Println("flushing NAT rules")
	cmd := exec.Command("iptables", "-F", "-t", "nat", "-v")
	_, err := cmd.Output()
	if err != nil {
		log.Fatal(err.Error())
	}
	cmd = exec.Command("iptables", "-F")
	_, err = cmd.Output()
	if err != nil {
		log.Fatal(err.Error())
	}
}

func disableReversePathFiltering(bridgeName string) {
	log.Println("enabling IP forwarding")
	cmd := exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1")
	_, err := cmd.Output()
	if err != nil {
		log.Fatal(err.Error())
	}
	cmd = exec.Command("echo", "0", ">", "/proc/sys/net/ipv4/conf/"+bridgeName+"/rp_filter")
	_, err = cmd.Output()
	if err != nil {
		log.Fatal(err.Error())
	}
	cmd = exec.Command("echo", "0", ">", "/proc/sys/net/ipv4/conf/"+bridgeName+"/rp_filter")
	_, err = cmd.Output()
	if err != nil {
		log.Fatal(err.Error())
	}
}

func enableForwarding(bridgeName string, proxyName string) {
	log.Println("enabling tun device forwarding")
	cmd := exec.Command("iptables", "-A", "FORWARD", "-i", bridgeName, "-o", proxyName, "-j", "ACCEPT")
	_, err := cmd.Output()
	cmd = exec.Command("iptables", "-A", "FORWARD", "-o", bridgeName, "-i", proxyName, "-j", "ACCEPT")
	if err == nil {
		_, err = cmd.Output()
	}
	cmd = exec.Command("iptables", "-A", "FORWARD", "-o", bridgeName, "-j", "ACCEPT")
	if err == nil {
		_, err = cmd.Output()
	}
	cmd = exec.Command("iptables", "-A", "FORWARD", "-i", bridgeName, "-j", "ACCEPT")
	if err == nil {
		_, err = cmd.Output()
	}
	if err != nil {
		log.Fatal(err.Error())
	}
}

func enableMasquerading(address string, mask string, bridgeName string) {
	log.Println("add NAT ip MASQUERADING")
	cmd := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", address+mask, "-o", bridgeName, "-j", "MASQUERADE")
	_, err := cmd.Output()
	if err != nil {
		log.Fatal(err.Error())
	}
}
