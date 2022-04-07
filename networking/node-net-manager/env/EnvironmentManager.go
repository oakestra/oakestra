package env

import (
	"NetManager/events"
	"NetManager/mqtt"
	"errors"
	"fmt"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
)

const NamespaceAlreadyDeclared string = "namespace already declared"

type EnvironmentManager interface {
	GetTableEntryByServiceIP(ip net.IP) []TableEntry
	GetTableEntryByNsIP(ip net.IP) (TableEntry, bool)
	GetTableEntryByInstanceIP(ip net.IP) (TableEntry, bool)
}

type Configuration struct {
	HostBridgeName             string
	HostBridgeIP               string
	HostBridgeMask             string
	HostTunName                string
	ConnectedInternetInterface string
	Mtusize                    int
}

type Environment struct {
	//### Environment management variables
	nodeNetwork       net.IPNet
	nameSpaces        []string
	networkInterfaces []networkInterface
	nextVethNumber    int
	proxyName         string
	config            Configuration
	translationTable  TableManager
	//### Deployment management variables
	deployedServices map[string]service //all the deployed services with the ip and ports
	nextContainerIP  net.IP             //next address for the next container to be deployed
	totNextAddr      int                //number of addresses currently generated, max 62
	addrCache        []net.IP           //Cache used to store the free addresses available for new containers
	//### Communication variables
	clusterPort string
	clusterAddr string
	mtusize     int
}

type service struct {
	ip          net.IP
	sname       string
	portmapping string
	veth        *netlink.Veth
}

// current network interfaces in the system
type networkInterface struct {
	number                   int
	veth0                    string
	veth0ip                  net.IP
	veth1                    string
	veth1ip                  net.IP
	isConnectedToAnInterface bool
	interfaceNumber          int
	namespace                string
}

type PortOperation string

const (
	OpenPorts  PortOperation = "-A"
	ClosePorts PortOperation = "-D"
)

// NewCustom environment constructor
func NewCustom(proxyname string, customConfig Configuration) *Environment {
	e := Environment{
		nameSpaces:        make([]string, 0),
		networkInterfaces: make([]networkInterface, 0),
		nextVethNumber:    0,
		proxyName:         proxyname,
		config:            customConfig,
		translationTable:  NewTableManager(),
		nextContainerIP:   nextIP(net.ParseIP(customConfig.HostBridgeIP), 1),
		totNextAddr:       1,
		addrCache:         make([]net.IP, 0),
		deployedServices:  make(map[string]service, 0),
		clusterAddr:       os.Getenv("CLUSTER_MANAGER_IP"),
		clusterPort:       os.Getenv("CLUSTER_MANAGER_PORT"),
		mtusize:           customConfig.Mtusize,
	}

	//Get Connected Internet Interface
	if e.config.ConnectedInternetInterface == "" {
		_, e.config.ConnectedInternetInterface = GetLocalIPandIface()
	}

	//create bridge
	log.Println("Creation of goProxyBridge")
	if err := e.CreateHostBridge(); err != nil {
		log.Fatal(err.Error())
	}

	//disable reverse path filtering
	log.Println("Disabling reverse path filtering")
	disableReversePathFiltering(e.config.HostBridgeName)

	//Enable tun device forwarding
	log.Println("Enabling packet forwarding")
	enableForwarding(e.config.HostBridgeName, proxyname)

	//Enable bridge MASQUERADING
	log.Println("Enabling packet masquerading")
	enableMasquerading(e.config.HostBridgeIP, e.config.HostBridgeMask, e.config.HostBridgeName, e.config.ConnectedInternetInterface)

	//update status with current network configuration
	log.Println("Reading the current environment configuration")

	return &e
}

// NewEnvironmentClusterConfigured Creates a new environment using the default configuration and asking the cluster for a new subnetwork
func NewEnvironmentClusterConfigured(proxyname string) *Environment {
	log.Println("Asking the cluster for a new subnetwork")
	subnetwork, err := mqtt.RequestSubnetworkMqttBlocking()
	if err != nil {
		log.Fatal("Invalid subnetwork received. Can't proceed.")
	}

	log.Println("Creating with default config")
	config := Configuration{
		HostBridgeName:             "goProxyBridge",
		HostBridgeIP:               nextIP(net.ParseIP(subnetwork), 1).String(),
		HostBridgeMask:             "/26",
		HostTunName:                "goProxyTun",
		ConnectedInternetInterface: "",
		Mtusize:                    3000,
	}
	return NewCustom(proxyname, config)
}

func (env *Environment) Destroy() {
	_ = netlink.LinkDel(&netlink.GenericLink{
		LinkAttrs: netlink.LinkAttrs{
			Name: env.config.HostBridgeName,
		},
	})
}

func (env *Environment) DetachContainer(sname string) {
	s, ok := env.deployedServices[sname]
	if ok {
		_ = env.translationTable.RemoveByNsip(s.ip)
		delete(env.deployedServices, sname)
		env.freeContainerAddress(s.ip)
		_ = env.manageContainerPorts(s.ip.String(), s.portmapping, ClosePorts)
		_ = netlink.LinkDel(s.veth)
	}
}

// ConfigureDockerNetwork creates a docker network compatible with the enviornment and returns it
func (env *Environment) ConfigureDockerNetwork(containername string) (string, error) {
	return "", errors.New("not yet implemented")
}

// AttachNetworkToContainer Attach a Docker container to the bridge and the current network environment
func (env *Environment) AttachNetworkToContainer(pid int, sname string, portmapping string) (net.IP, error) {

	cleanup := func(veth *netlink.Veth) {
		_ = netlink.LinkDel(veth)
	}

	vethIfce, err := env.createVethsPairAndAttachToBridge(sname, env.mtusize)
	if err != nil {
		cleanup(vethIfce)
		return nil, err
	}

	// Attach veth2 to the docker container
	log.Println("Attaching peerveth to container ")
	peerVeth, err := netlink.LinkByName(vethIfce.PeerName)
	if err != nil {
		cleanup(vethIfce)
		return nil, err
	}
	if err := netlink.LinkSetNsPid(peerVeth, pid); err != nil {
		cleanup(vethIfce)
		return nil, err
	}

	//generate a new ip for this container
	ip, err := env.generateAddress()
	if err != nil {
		cleanup(vethIfce)
		return nil, err
	}

	// set ip to the container veth
	log.Println("Assigning ip ", ip.String()+env.config.HostBridgeMask, " to container ")
	if err := env.addPeerLinkNetwork(pid, ip.String()+env.config.HostBridgeMask, vethIfce.PeerName); err != nil {
		cleanup(vethIfce)
		env.freeContainerAddress(ip)
		return nil, err
	}

	//Add traffic route to bridge
	log.Println("Setting container routes ")
	if err = env.setContainerRoutes(pid, vethIfce.PeerName); err != nil {
		cleanup(vethIfce)
		env.freeContainerAddress(ip)
		return nil, err
	}

	env.BookVethNumber()

	if err = env.setVethFirewallRules(vethIfce.Name); err != nil {
		env.freeContainerAddress(ip)
		cleanup(vethIfce)
		return nil, err
	}

	if err = env.manageContainerPorts(ip.String(), portmapping, OpenPorts); err != nil {
		debug.PrintStack()
		env.freeContainerAddress(ip)
		cleanup(vethIfce)
		return nil, err
	}

	env.deployedServices[sname] = service{
		ip:          ip,
		sname:       sname,
		portmapping: portmapping,
		veth:        vethIfce,
	}
	return ip, nil
}

//create veth pair and connect one to the host bridge
//returns: bridgeVeth name, free Veth name, Vether interface to the veth pair and eventually an error
func (env *Environment) createVethsPairAndAttachToBridge(sname string, mtu int) (*netlink.Veth, error) {
	// Retrieve current bridge
	bridge, err := netlink.LinkByName(env.config.HostBridgeName)
	if err != nil {
		return nil, err
	}
	hashedName := NameUniqueHash(sname, 4)
	veth1name := fmt.Sprintf("veth%s%s%s", "00", strconv.Itoa(env.nextVethNumber), hashedName)
	veth2name := fmt.Sprintf("veth%s%s%s", "01", strconv.Itoa(env.nextVethNumber), hashedName)
	log.Println("creating veth pair: " + veth1name + "@" + veth2name)

	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: veth1name,
			MTU:  mtu},
		PeerName: veth2name,
	}
	err = netlink.LinkAdd(veth)
	if err != nil {
		return nil, err
	}

	// add veth1 to the bridge
	err = netlink.LinkSetMaster(veth, bridge)
	if err != nil {
		return nil, err
	}

	// set veth status up
	if err = netlink.LinkSetUp(veth); err != nil {
		return nil, err
	}

	return veth, nil
}

// sets the FPRWARD firewall rules for the bridge veth
func (env *Environment) setVethFirewallRules(bridgeVethName string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// iptables -A FORWARD -o goProxyBridge -i veth -j ACCEPT
	cmd := exec.Command("iptables", "-A", "FORWARD", "-o", env.config.HostBridgeName, "-i", bridgeVethName, "-j", "ACCEPT")
	err := cmd.Run()
	if err != nil {
		return err
	}
	cmd = exec.Command("iptables", "-A", "FORWARD", "-i", env.config.HostBridgeName, "-o", bridgeVethName, "-j", "ACCEPT")
	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

// add routes inside the container namespace to forward the traffic using the bridge
func (env *Environment) setContainerRoutes(containerPid int, peerVeth string) error {
	//Add route to bridge
	//sudo nsenter -n -t 5565 ip route add 0.0.0.0/0 via 127.19.x.y dev veth013
	err := env.execInsideNs(containerPid, func() error {
		link, err := netlink.LinkByName(peerVeth)
		if err != nil {
			return err
		}
		dst, err := netlink.ParseIPNet("0.0.0.0/0")
		if err != nil {
			return err
		}
		gw := net.ParseIP(env.config.HostBridgeIP)
		return netlink.RouteAdd(&netlink.Route{
			LinkIndex: link.Attrs().Index,
			Dst:       dst,
			Gw:        gw,
		})
	})
	if err != nil {
		log.Printf("Impossible to setup route inside the netns: %v\n", err)
		return err
	}
	return nil
}

// setup the address of the network namespace veth
func (env *Environment) addPeerLinkNetwork(nspid int, addr string, vethname string) error {
	netlinkAddr, err := netlink.ParseAddr(addr)
	if err != nil {
		return err
	}
	err = env.execInsideNs(nspid, func() error {
		link, err := netlink.LinkByName(vethname)
		if err != nil {
			return err
		}
		err = netlink.AddrAdd(link, netlinkAddr)
		if err == nil {
			err = netlink.LinkSetUp(link)
		}
		return err
	})
	if err != nil {
		return err
	}
	return err
}

// Execute function inside a namespace
func (env *Environment) execInsideNs(pid int, function func() error) error {
	var containerNs netns.NsHandle

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	stdNetns, err := netns.Get()
	if err == nil {
		defer stdNetns.Close()
		containerNs, err = netns.GetFromPid(pid)
		if err == nil {
			defer netns.Set(stdNetns)
			err = netns.Set(containerNs)
			if err == nil {
				err = function()
			}
		}
	}
	return err
}

// BookVethNumber Update the veth number to be used for the next veth
func (env *Environment) BookVethNumber() {
	env.nextVethNumber = env.nextVethNumber + 1
}

// CreateHostBridge create host bridge if it has not been created yet, return the current host bridge name or the newly created one
func (env *Environment) CreateHostBridge() error {
	//check current declared bridges
	bridgecmd := exec.Command("ip", "link", "list", "type", "bridge")
	bridgelines, err := bridgecmd.Output()
	if err != nil {
		return err
	}
	currentDeclaredBridges := extractNetInterfaceName(string(bridgelines))

	//is HostBridgeName already created? DESTROY IT
	for _, name := range currentDeclaredBridges {
		if name == env.config.HostBridgeName {
			log.Println("Removing previous bridge")
			env.Destroy()
		}
	}

	//otherwise create it
	log.Printf("Creating new bridge: %s\n", env.config.HostBridgeName)
	createbridgeCmd := exec.Command("ip", "link", "add", "name", env.config.HostBridgeName, "mtu", strconv.Itoa(env.mtusize), "type", "bridge")
	_, err = createbridgeCmd.Output()
	if err != nil {
		return err
	}

	//assign ip to the bridge
	log.Println("Assigning IP to the new bridge")
	bridgeIpCmd := exec.Command("ip", "a", "add",
		env.config.HostBridgeIP+env.config.HostBridgeMask, "dev", env.config.HostBridgeName)
	_, err = bridgeIpCmd.Output()
	if err != nil {
		return err
	}

	//bring the bridge up
	log.Println("Setting bridge UP")
	bridgeUpCmd := exec.Command("ip", "link", "set", "dev", env.config.HostBridgeName, "up")
	_, err = bridgeUpCmd.Output()
	if err != nil {
		return err
	}

	return nil
}

// GetTableEntryByServiceIP Given a ServiceIP this method performs a search in the local ServiceCache
//If the entry is not present a TableQuery is performed and the interest registered
func (env *Environment) GetTableEntryByServiceIP(ip net.IP) []TableEntry {
	//If entry already available
	table := env.translationTable.SearchByServiceIP(ip)
	if len(table) > 0 {
		//Fire table instance usage event
		events.GetInstance().Emit(events.Event{
			EventType:   events.TableQuery,
			EventTarget: table[0].Servicename,
		})
		return table
	}

	//if no entry available -> TableQuery
	entryList, err := tableQueryByIP(ip.String())

	if err == nil {
		var once sync.Once
		for _, tableEntry := range entryList {
			once.Do(func() { mqtt.MqttRegisterInterest(tableEntry.JobName, env) })
			env.AddTableQueryEntry(tableEntry)
		}
		table = env.translationTable.SearchByServiceIP(ip)
	}

	return table
}

// GetTableEntryByInstanceIP Given a ServiceIP this method performs a search in the local ServiceCache
//If the entry is not present a TableQuery is performed and the interest registered
func (env *Environment) GetTableEntryByInstanceIP(ip net.IP) (TableEntry, bool) {
	//If entry already available
	table := env.translationTable.SearchByServiceIP(ip)
	if len(table) > 0 {
		for elemindex, elem := range table {
			for _, elemIp := range elem.ServiceIP {
				if elemIp.IpType == InstanceNumber && elemIp.Address.Equal(ip) {
					return table[elemindex], true
				}
			}
		}
	}
	return TableEntry{}, false
}

// GetTableEntryByNsIP Given a NamespaceIP finds the table entry. This search is local because the networking component MUST have all
//the entries for the local deployed services.
func (env *Environment) GetTableEntryByNsIP(ip net.IP) (TableEntry, bool) {
	//If entry already available
	entry, exist := env.translationTable.SearchByNsIP(ip)
	if exist {
		return entry, true
	}
	return entry, false
}

// AddTableQueryEntry Add new entry to the resolution table
func (env *Environment) AddTableQueryEntry(entry TableEntry) {
	_ = env.translationTable.RemoveByNsip(entry.Nsip)
	err := env.translationTable.Add(entry)
	if err != nil {
		log.Println("[ERROR] ", err)
	}
}

// RefreshServiceTable force a table query refresh for a service
func (env *Environment) RefreshServiceTable(jobname string) {
	log.Printf("Requested table query refresh fro %s", jobname)
	entryList, err := tableQueryByJobName(jobname)
	_ = env.translationTable.RemoveByJobName(jobname)
	if err == nil {
		for _, tableEntry := range entryList {
			env.AddTableQueryEntry(tableEntry)
		}
	}
}

func (env *Environment) RemoveServiceEntries(jobname string) {
	_ = env.translationTable.RemoveByJobName(jobname)
}

func (env *Environment) RemoveNsIPEntries(nsip string) {
	_ = env.translationTable.RemoveByNsip(net.IP(nsip))
}

func (env *Environment) generateAddress() (net.IP, error) {
	var result net.IP
	if len(env.addrCache) > 0 {
		result, env.addrCache = env.addrCache[0], env.addrCache[1:]
	} else {
		result = env.nextContainerIP
		if env.totNextAddr < 62 {
			env.totNextAddr++
		} else {
			log.Println("[ERROR] exhausted address space")
			return result, errors.New("address space exhausted")
		}
		env.nextContainerIP = nextIP(env.nextContainerIP, 1)
	}
	return result, nil
}

func (env *Environment) freeContainerAddress(ip net.IP) {
	env.addrCache = append(env.addrCache, ip)
}

func (env *Environment) manageContainerPorts(localContainerAddress string, portmapping string, operation PortOperation) error {
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
		openPortCmd := exec.Command("iptables", "-t", "nat", string(operation), "PREROUTING", "-p", portType, "--dport", hostPort, "-j", "DNAT", "--to-destination", destination)
		status, err := openPortCmd.Output()
		if err != nil {
			log.Printf("ERROR: %s\n", string(status))
			return err
		}
		log.Printf("Changed port %s status toward destination %s\n", hostPort, destination)
	}
	return nil
}
