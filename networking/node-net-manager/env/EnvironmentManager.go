package env

import (
	"NetManager/TableEntryCache"
	"NetManager/events"
	"NetManager/logger"
	"NetManager/mqtt"
	"NetManager/network"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

const NamespaceAlreadyDeclared string = "namespace already declared"

type EnvironmentManager interface {
	GetTableEntryByServiceIP(ip net.IP) []TableEntryCache.TableEntry
	GetTableEntryByNsIP(ip net.IP) (TableEntryCache.TableEntry, bool)
	GetTableEntryByInstanceIP(ip net.IP) (TableEntryCache.TableEntry, bool)
}

type Configuration struct {
	HostBridgeName             string
	HostBridgeIP               string
	HostBridgeMask             string
	HostBridgeIPv6             string
	HostBridgeIPv6Prefix       string
	HostTunName                string
	ConnectedInternetInterface string
	Mtusize                    int
}

type Environment struct {
	//### Environment management variables
	nodeNetwork       net.IPNet
	nodeNetworkv6     net.IPNet
	nameSpaces        []string
	networkInterfaces []networkInterface
	nextVethNumber    int
	proxyName         string
	config            Configuration
	translationTable  TableEntryCache.TableManager
	//### Deployment management variables
	deployedServices     map[string]service // all the deployed services with the ip and ports
	deployedServicesLock sync.RWMutex
	nextContainerIP      net.IP // next address for the next container to be deployed
	nextContainerIPv6    net.IP
	totNextAddr          int // number of addresses currently generated, max 62
	totNextAddrv6        int
	addrCache            []net.IP // Cache used to store the free addresses available for new containers
	addrCachev6          []net.IP
	//### Communication variables
	clusterPort string
	clusterAddr string
	mtusize     int
}

type service struct {
	ip          net.IP
	ipv6        net.IP
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

// NewCustom environment constructor
func NewCustom(proxyname string, customConfig Configuration) *Environment {
	e := Environment{
		nameSpaces:        make([]string, 0),
		networkInterfaces: make([]networkInterface, 0),
		nextVethNumber:    0,
		proxyName:         proxyname,
		config:            customConfig,
		translationTable:  TableEntryCache.NewTableManager(),
		nextContainerIP:   network.NextIPv4(net.ParseIP(customConfig.HostBridgeIP), 1),
		nextContainerIPv6: network.NextIPv6(net.ParseIP(customConfig.HostBridgeIPv6), 1),
		totNextAddr:       1,
		totNextAddrv6:     1,
		addrCache:         make([]net.IP, 0),
		addrCachev6:       make([]net.IP, 0),
		deployedServices:  make(map[string]service, 0),
		clusterAddr:       os.Getenv("CLUSTER_MANAGER_IP"),
		clusterPort:       os.Getenv("CLUSTER_MANAGER_PORT"),
		mtusize:           customConfig.Mtusize,
	}

	// Get Connected Internet Interface
	if e.config.ConnectedInternetInterface == "" {
		_, e.config.ConnectedInternetInterface = network.GetLocalIPandIface()
		logger.InfoLogger().Println(e.config.ConnectedInternetInterface)

	}

	// create bridge
	logger.InfoLogger().Println("Creation of goProxyBridge")
	if err := e.CreateHostBridge(); err != nil {
		log.Fatal(err)
	}

	// disable reverse path filtering
	logger.InfoLogger().Println("Disabling reverse path filtering")
	network.DisableReversePathFiltering(e.config.HostBridgeName)

	// Enable tun device forwarding
	logger.InfoLogger().Println("Enabling packet forwarding")
	network.EnableForwarding(e.config.HostBridgeName, proxyname)

	// Enable bridge MASQUERADING
	logger.InfoLogger().Println("Enabling packet masquerading")
	network.EnableMasquerading(e.config.HostBridgeIP, e.config.HostBridgeMask, e.config.HostBridgeIPv6, e.config.HostBridgeIPv6Prefix, e.config.HostBridgeName, e.config.ConnectedInternetInterface)

	// update status with current network configuration
	logger.InfoLogger().Println("Reading the current environment configuration")

	return &e
}

// NewEnvironmentClusterConfigured Creates a new environment using the default configuration and asking the cluster for a new subnetwork
func NewEnvironmentClusterConfigured(proxyname string) *Environment {
	logger.InfoLogger().Println("Asking the cluster for a new subnetwork")
	subnetwork_response, err := mqtt.RequestSubnetworkMqttBlocking()
	if err != nil {
		log.Fatal("Invalid subnetwork received. Can't proceed.")
	}
	ipv4_subnet := subnetwork_response.Address
	ipv6_subnet := subnetwork_response.Address_v6

	logger.InfoLogger().Println("Creating with default config")
	mtusize, err := strconv.Atoi(os.Getenv("TUN_MTU_SIZE"))
	if mtusize < 0 || err != nil {
		logger.InfoLogger().Printf("Default to mtusize 1450")
		mtusize = 1450
	}
	config := Configuration{
		HostBridgeName:             "goProxyBridge",
		HostBridgeIP:               network.NextIPv4(net.ParseIP(ipv4_subnet), 1).String(),
		HostBridgeMask:             "/26",
		HostBridgeIPv6:             network.NextIPv6(net.ParseIP(ipv6_subnet), 1).String(),
		HostBridgeIPv6Prefix:       "/120",
		HostTunName:                "goProxyTun",
		ConnectedInternetInterface: "",
		Mtusize:                    mtusize,
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

func (env *Environment) IsServiceDeployed(jobName string) bool {
	env.deployedServicesLock.RLock()
	defer env.deployedServicesLock.RUnlock()
	for _, element := range env.deployedServices {
		if element.sname == jobName {
			return true
		}
	}
	return false
}

// ConfigureDockerNetwork creates a docker network compatible with the enviornment and returns it
func (env *Environment) ConfigureDockerNetwork(containername string) (string, error) {
	return "", errors.New("not yet implemented")
}

// create veth pair and connect one to the host bridge
// returns: bridgeVeth name, free Veth name, Vether interface to the veth pair and eventually an error
func (env *Environment) createVethsPairAndAttachToBridge(sname string, mtu int) (*netlink.Veth, error) {
	// Retrieve current bridge
	logger.DebugLogger().Println("Retrieving current bridge ")
	bridge, err := netlink.LinkByName(env.config.HostBridgeName)
	if err != nil {
		logger.ErrorLogger().Println("Error retrieving current bridge: ", err)
		return nil, err
	}
	logger.DebugLogger().Println("Retrieved current bridge")
	hashedName := network.NameUniqueHash(sname, 4)
	veth1name := fmt.Sprintf("veth%s%s%s", "00", strconv.Itoa(env.nextVethNumber), hashedName)
	veth2name := fmt.Sprintf("veth%s%s%s", "01", strconv.Itoa(env.nextVethNumber), hashedName)
	logger.DebugLogger().Println("creating veth pair: " + veth1name + "@" + veth2name)

	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: veth1name,
			MTU:  mtu,
		},
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

// sets the FORWARD firewall rules for the bridge veth
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
	// Add route to bridge
	// sudo nsenter -n -t 5565 ip route add 0.0.0.0/0 via 127.19.x.y dev veth013
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
		logger.ErrorLogger().Printf("Impossible to setup route inside the netns: %v\n", err)
		return err
	}
	return nil
}

func (env *Environment) setIPv6ContainerRoutes(containerPid int, peerVeth string) error {
	err := env.execInsideNs(containerPid, func() error {
		link, err := netlink.LinkByName(peerVeth)
		if err != nil {
			return err
		}
		dstv6, err := netlink.ParseIPNet("::/0")
		if err != nil {
			return err
		}
		gwv6 := net.ParseIP(env.config.HostBridgeIPv6)
		return netlink.RouteAdd(&netlink.Route{
			LinkIndex: link.Attrs().Index,
			Dst:       dstv6,
			Gw:        gwv6,
		})
	})
	if err != nil {
		logger.ErrorLogger().Printf("Impossible to setup IPv6 route inside the netns: %v\n", err)
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

// setup the address of the network namespace veth based on Ns name
func (env *Environment) addPeerLinkNetworkByNsName(NsName string, addr string, vethname string) error {
	netlinkAddr, err := netlink.ParseAddr(addr)
	if err != nil {
		return err
	}
	err = env.execInsideNsByName(NsName, func() error {
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
	return err
}

// disable Duplicate Address Detection (DAD) for IPv6 interfaces in namespace
// to prevent interface startup delay
func (env *Environment) disableDAD(pid int, vethname string) error {
	err := env.execInsideNs(pid, func() error {
		cmd := exec.Command("sysctl", "-w", "net.ipv6.conf.default.accept_dad=0")
		err := cmd.Run()
		if err != nil {
			return err
		}
		cmd = exec.Command("sysctl", "-w", "net.ipv6.conf."+vethname+".accept_dad=0")
		err = cmd.Run()
		return err
	})
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

// Execute function inside a namespace based on Ns name
func (env *Environment) execInsideNsByName(Nsname string, function func() error) error {
	var containerNs netns.NsHandle

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	stdNetns, err := netns.Get()
	if err == nil {
		defer stdNetns.Close()
		containerNs, err = netns.GetFromName(Nsname)
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
	// check current declared bridges
	devices, err := net.Interfaces()
	if err != nil {
		return err
	}

	// is HostBridgeName already created? DESTROY IT
	for _, ifce := range devices {
		if ifce.Name == env.config.HostBridgeName {
			logger.DebugLogger().Println("Removing previous bridge")
			env.Destroy()
		}
	}

	// otherwise create it
	logger.DebugLogger().Printf("Creating new bridge: %s\n", env.config.HostBridgeName)
	createbridgeCmd := exec.Command("ip", "link", "add", "name", env.config.HostBridgeName, "mtu", strconv.Itoa(env.mtusize), "type", "bridge")
	_, err = createbridgeCmd.Output()
	if err != nil {
		return err
	}

	// assign ip to the bridge
	logger.DebugLogger().Println("Assigning IPv4 to the new bridge")
	bridgeIpCmd := exec.Command("ip", "a", "add",
		env.config.HostBridgeIP+env.config.HostBridgeMask, "dev", env.config.HostBridgeName)
	_, err = bridgeIpCmd.Output()
	if err != nil {
		return err
	}

	logger.DebugLogger().Println("Assigning IPv6 to the new bridge")
	bridgeIpv6Cmd := exec.Command("ip", "a", "add",
		env.config.HostBridgeIPv6+env.config.HostBridgeIPv6Prefix, "dev", env.config.HostBridgeName)
	_, err = bridgeIpv6Cmd.Output()
	if err != nil {
		return err
	}

	// bring the bridge up
	logger.DebugLogger().Println("Setting bridge UP")
	bridgeUpCmd := exec.Command("ip", "link", "set", "dev", env.config.HostBridgeName, "up")
	_, err = bridgeUpCmd.Output()
	if err != nil {
		return err
	}

	return nil
}

// GetTableEntryByServiceIP Given a ServiceIP this method performs a search in the local ServiceCache
// If the entry is not present a TableQuery is performed and the interest registered
func (env *Environment) GetTableEntryByServiceIP(ip net.IP) []TableEntryCache.TableEntry {
	// If entry already available
	table := env.translationTable.SearchByServiceIP(ip)
	if len(table) > 0 {
		// Fire table instance usage event
		events.GetInstance().Emit(events.Event{
			EventType:   events.TableQuery,
			EventTarget: table[0].JobName,
		})
		return table
	}

	// if no entry available -> TableQuery
	entryList, err := tableQueryByIP(ip)

	if err == nil {
		var once sync.Once
		for _, tableEntry := range entryList {
			once.Do(func() { mqtt.MqttRegisterInterest(tableEntry.JobName, env) })
			env.AddTableQueryEntry(tableEntry)
		}
		table = env.translationTable.SearchByServiceIP(ip)
		// register interest for sip as well to avoid querying the address too many times
		mqtt.MqttRegisterInterest(ip.String(), env)
	}

	return table
}

// GetTableEntryByInstanceIP Given a ServiceIP this method performs a search in the local ServiceCache
// If the entry is not present a TableQuery is performed and the interest registered
func (env *Environment) GetTableEntryByInstanceIP(ip net.IP) (TableEntryCache.TableEntry, bool) {
	// If entry already available
	table := env.translationTable.SearchByServiceIP(ip)
	if len(table) > 0 {
		for elemindex, elem := range table {
			for _, elemIp := range elem.ServiceIP {
				if elemIp.IpType == TableEntryCache.InstanceNumber &&
					(elemIp.Address.Equal(ip) || elemIp.Address_v6.Equal(ip)) {
					return table[elemindex], true
				}
			}
		}
	}
	return TableEntryCache.TableEntry{}, false
}

// GetTableEntryByNsIP Given a NamespaceIP finds the table entry. This search is local because the networking component MUST have all
// the entries for the local deployed services.
func (env *Environment) GetTableEntryByNsIP(ip net.IP) (TableEntryCache.TableEntry, bool) {
	// If entry already available
	entry, exist := env.translationTable.SearchByNsIP(ip)
	if exist {
		return entry, true
	}
	return entry, false
}

// AddTableQueryEntry Add new entry to the resolution table
func (env *Environment) AddTableQueryEntry(entry TableEntryCache.TableEntry) {
	_ = env.translationTable.RemoveByNsip(entry.Nsip)
	err := env.translationTable.Add(entry)
	if err != nil {
		logger.ErrorLogger().Println(err)
	}
}

// RefreshServiceTable force a table query refresh for a service
func (env *Environment) RefreshServiceTable(jobname string) {
	logger.DebugLogger().Printf("Requested table query refresh for %s", jobname)
	entryList, err := tableQueryByJobName(jobname, true)
	if err == nil {
		_ = env.translationTable.RemoveByJobName(jobname)
		for _, tableEntry := range entryList {
			env.AddTableQueryEntry(tableEntry)
		}
	}
}

func (env *Environment) RemoveServiceEntries(jobname string) {
	err := env.translationTable.RemoveByJobName(jobname)
	if err != nil {
		logger.ErrorLogger().Printf("CRITICAL-ERROR: %v", err)
	}
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
			logger.ErrorLogger().Printf("exhausted IPv4 address space")
			return result, errors.New("IPv4 address space exhausted")
		}
		env.nextContainerIP = network.NextIPv4(env.nextContainerIP, 1)
	}
	return result, nil
}

func (env *Environment) generateIPv6Address() (net.IP, error) {
	var result net.IP
	if len(env.addrCachev6) > 0 {
		result, env.addrCachev6 = env.addrCachev6[0], env.addrCachev6[1:]
	} else {
		result = env.nextContainerIPv6
		if env.totNextAddrv6 < 255 {
			env.totNextAddrv6++
		} else {
			logger.ErrorLogger().Printf("exhausted IPv6 address space")
			return result, errors.New("IPv6 address space exhausted")
		}
		env.nextContainerIPv6 = network.NextIPv6(env.nextContainerIPv6, 1)
	}
	return result, nil
}

func (env *Environment) freeContainerAddress(ip net.IP) {
	// if ip is an IPv4 addr
	if err := ip.To4(); err != nil {
		env.addrCache = append(env.addrCache, ip)
	} else
	// else check whether it is a correct IPv6 address
	// this cannot be an IPv4-to-IPv6 mapped IPv6 addr, as we handle IPv4 beforehand
	if err = ip.To16(); err != nil {
		env.addrCachev6 = append(env.addrCachev6, ip)
	}
}
