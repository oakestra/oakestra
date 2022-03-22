package env

import (
	"log"
	"os/exec"
)

func IptableFlushAll() {
	log.Println("flushing NAT rules")
	cmd := exec.Command("iptables", "-F", "-t", "nat", "-v")
	err := cmd.Run()
	if err != nil {
		log.Fatal(err.Error())
	}
	cmd = exec.Command("iptables", "-F")
	err = cmd.Run()
	if err != nil {
		log.Fatal(err.Error())
	}
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
	cmd := exec.Command("iptables", "-A", "FORWARD", "-i", bridgeName, "-o", proxyName, "-j", "ACCEPT")
	err := cmd.Run()
	cmd = exec.Command("iptables", "-A", "FORWARD", "-o", bridgeName, "-i", proxyName, "-j", "ACCEPT")
	if err == nil {
		err = cmd.Run()
	}
	cmd = exec.Command("iptables", "-A", "FORWARD", "-o", bridgeName, "-j", "ACCEPT")
	if err == nil {
		err = cmd.Run()
	}
	cmd = exec.Command("iptables", "-A", "FORWARD", "-i", bridgeName, "-j", "ACCEPT")
	if err == nil {
		err = cmd.Run()
	}
	if err != nil {
		log.Fatal(err.Error())
	}
}

func enableMasquerading(address string, mask string, bridgeName string, internetIfce string) {
	log.Println("add NAT ip MASQUERADING")
	cmd := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", address+mask, "-o", bridgeName, "-j", "MASQUERADE")
	err := cmd.Run()
	if err != nil {
		log.Fatal(err.Error())
	}

	log.Println("add NAT ip MASQUERADING for the bridge")
	cmd = exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", address+mask, "-o", internetIfce, "-j", "MASQUERADE")
	err = cmd.Run()
	if err != nil {
		log.Fatal(err.Error())
	}
}
