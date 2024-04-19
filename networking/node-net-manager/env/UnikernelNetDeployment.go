package env

import (
	"NetManager/logger"
	"NetManager/network"
	"fmt"
	"net"
	"os/exec"
	"runtime/debug"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

type UnikernelDeyplomentHandler struct {
	env *Environment
}

var unikernelHandler *UnikernelDeyplomentHandler = nil

func GetUnikernelNetDeployment() *UnikernelDeyplomentHandler {
	if unikernelHandler == nil {
		logger.ErrorLogger().Fatal("Unikernel Handler not initialized")
	}
	return unikernelHandler
}

func InitUnikernelDeployment(env *Environment) {
	unikernelHandler = &UnikernelDeyplomentHandler{
		env: env,
	}
}

func (h *UnikernelDeyplomentHandler) DeployNetwork(pid int, sname string, instancenumber int, portmapping string) (net.IP, net.IP, error) {
	env := h.env
	name := sname
	sname = fmt.Sprintf("%s.instance.%d", sname, instancenumber)

	cleanup := func(veth *netlink.Veth) {
		_ = netlink.LinkDel(veth)
	}

	logger.DebugLogger().Println("Creating veth pair for unikernel deployment")
	vethIfce, err := env.createVethsPairAndAttachToBridge(sname, env.mtusize)
	if err != nil {
		cleanup(vethIfce)
		return nil, nil, err
	}

	peerVeth, err := netlink.LinkByName(vethIfce.PeerName)
	if err != nil {
		cleanup(vethIfce)
		return nil, nil, err
	}

	logger.DebugLogger().Printf("Creating Namespace for unikernel (%s)", sname)
	nscreation := exec.Command("ip", "netns", "add", sname)
	err = nscreation.Run()
	// ns, err := netns.NewNamed(sname) ## Changes Namespace of current application
	if err != nil {
		cleanup(vethIfce)
		return nil, nil, err
	}
	ns, err := netns.GetFromName(sname)
	if err != nil {
		logger.DebugLogger().Printf("Unable to find namespace: %v", err)
		return nil, nil, err
	}

	cleanup = func(veth *netlink.Veth) {
		_ = netlink.LinkDel(veth)
		ns.Close()
		err = netns.DeleteNamed(sname)
		if err != nil {
			logger.DebugLogger().Printf("Unable to delete namespace: %v", err)
		}
	}

	if err := netlink.LinkSetNsFd(peerVeth, int(ns)); err != nil {
		logger.DebugLogger().Printf("Error %s: %v", peerVeth.Attrs().Name, err)
		cleanup(vethIfce)
		return nil, nil, err
	}

	// Get IP for veth interface
	ip, err := env.generateAddress()
	if err != nil {
		cleanup(vethIfce)
		return nil, nil, err
	}

	if err := env.addPeerLinkNetworkByNsName(sname, ip.String()+env.config.HostBridgeMask, vethIfce.PeerName); err != nil {
		logger.DebugLogger().Println("Unable to configure Peer")
		cleanup(vethIfce)
		env.freeContainerAddress(ip)
		return nil, nil, err
	}

	// Create Bridge and tap within Ns
	logger.DebugLogger().Println("Creating Bridge and Tap inside of Ns")
	labr := netlink.NewLinkAttrs()
	labr.Name = "virbr0"
	bridge := &netlink.Bridge{LinkAttrs: labr}
	lat := netlink.NewLinkAttrs()
	lat.Name = "tap0"
	tap := &netlink.Tuntap{LinkAttrs: lat, Mode: netlink.TUNTAP_MODE_TAP}
	err = env.execInsideNsByName(sname, func() error {
		// Create Bridge
		err := netlink.LinkAdd(bridge)
		if err != nil {
			logger.DebugLogger().Printf("Unable to create Bridge: %v\n", err)
			return err
		}
		// Set IP on Bridge
		addrbr, _ := netlink.ParseAddr("192.168.1.1/30")
		err = netlink.AddrAdd(bridge, addrbr)
		if err != nil {
			logger.DebugLogger().Printf("Unable to add ip address to bridge: %v\n", err)
			return err
		}
		// Create tap for Qemu
		err = netlink.LinkAdd(tap)
		if err != nil {
			logger.DebugLogger().Printf("Unable to create Tap: %v\n", err)
			return err
		}
		// Attach tap to Bridge
		if netlink.LinkSetMaster(tap, bridge) != nil {
			logger.DebugLogger().Printf("Unable to set master to tap: %v\n", err)
			return err
		}

		// ip link set up virbr0/tap0
		cmd := exec.Command("ip", "link", "set", "up", "dev", "virbr0")
		err = cmd.Run()
		if err != nil {
			return err
		}
		cmd = exec.Command("ip", "link", "set", "up", "dev", "tap0")
		err = cmd.Run()
		if err != nil {
			return err
		}

		// Set route for Ns
		dst, err := netlink.ParseIPNet("0.0.0.0/0")
		if err != nil {
			return err
		}

		err = netlink.RouteAdd(&netlink.Route{
			LinkIndex: peerVeth.Attrs().Index,
			Dst:       dst,
			Gw:        net.ParseIP(env.config.HostBridgeIP),
		})
		if err != nil {
			logger.DebugLogger().Printf("Failed to set route in Ns: %v", err)
			return err
		}

		// Set NAT for Unikernel
		cmd = exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-o", vethIfce.PeerName, "-j", "SNAT", "--to", ip.String())
		err = cmd.Run()
		if err != nil {
			return err
		}
		cmd = exec.Command("iptables", "-t", "nat", "-A", "PREROUTING", "-i", vethIfce.PeerName, "-j", "DNAT", "--to", "192.168.1.2")
		err = cmd.Run()
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		logger.DebugLogger().Printf("Failed to configure Ns for Unikernel\n")
		cleanup(vethIfce)
		env.freeContainerAddress(ip)
		return nil, nil, err
	}

	env.BookVethNumber()

	if err = env.setVethFirewallRules(vethIfce.Name); err != nil {
		cleanup(vethIfce)
		env.freeContainerAddress(ip)
		return nil, nil, err
	}

	if err = network.ManageContainerPorts(ip, portmapping, network.OpenPorts); err != nil {
		debug.PrintStack()
		cleanup(vethIfce)
		env.freeContainerAddress(ip)
		return nil, nil, err
	}

	env.deployedServicesLock.Lock()
	env.deployedServices[sname] = service{
		ip:          ip,
		sname:       name,
		portmapping: portmapping,
		veth:        vethIfce,
	}
	env.deployedServicesLock.Unlock()
	logger.DebugLogger().Println("Successful Network creation for Unikernel")
	return ip, nil, nil
}

func (env *Environment) DeleteUnikernelNamespace(sname string, instance int) {
	name := fmt.Sprintf("%s.instance.%d", sname, instance)
	s, ok := env.deployedServices[name]
	if ok {
		_ = env.translationTable.RemoveByNsip(s.ip)
		env.deployedServicesLock.Lock()
		delete(env.deployedServices, name)
		env.deployedServicesLock.Unlock()
		env.freeContainerAddress(s.ip)
		_ = network.ManageContainerPorts(s.ip, s.portmapping, network.ClosePorts)
		_ = netlink.LinkDel(s.veth)
		_ = netns.DeleteNamed(name)
	}
}
