package env

import (
	"NetManager/events"
	"NetManager/mqtt"
	"errors"
	"fmt"
	"github.com/milosgajdos/tenus"
	"github.com/tkanos/gonfig"
	"log"
	"net"
	"os"
	"os/exec"
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
	Mtusize                    string
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
	mtusize     string
}

type service struct {
	ip          net.IP
	sname       string
	portmapping map[int]int
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
func NewCustom(proxyname string, customConfig Configuration) Environment {
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

	err := e.Update()
	if err == nil {
		e.Destroy()
	}

	//create bridge
	log.Println("Creation of goProxyBridge")
	_, err = e.CreateHostBridge()
	if err != nil {
		log.Fatal(err.Error())
	}
	e.nextVethNumber = 0

	//flush current nat rules
	//iptables -F -t nat -v
	iptableFlushAll()

	//disable reverse path filtering
	disableReversePathFiltering(e.config.HostBridgeName)

	//Enable tun device forwarding
	enableForwarding(e.config.HostBridgeName, proxyname)

	//Enable bridge MASQUERADING
	enableMasquerading(e.config.HostBridgeIP, e.config.HostBridgeMask, e.config.HostBridgeName)

	//update status with current network configuration
	log.Println("Reading the current environment configuration")
	err = e.Update()
	if err != nil {
		log.Fatal(err.Error())
	}

	return e
}

// NewStatic Creates a new environment using the static configuration files
func NewStatic(proxyname string) Environment {
	log.Println("Loading config file for environment creation")
	config := Configuration{}
	//parse confgiuration file
	err := gonfig.GetConf("config/envcfg.json", &config)
	if err != nil {
		log.Fatal(err)
	}
	return NewCustom(proxyname, config)
}

// NewEnvironmentClusterConfigured Creates a new environment using the default configuration and asking the cluster for a new subnetwork
func NewEnvironmentClusterConfigured(proxyname string) Environment {
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
		Mtusize:                    "3000",
	}
	return NewCustom(proxyname, config)
}

func (env *Environment) DetachContainer(sname string) {
	s, ok := env.deployedServices[sname]
	if ok {
		_ = env.translationTable.RemoveByNsip(s.ip)
		delete(env.deployedServices, sname)
		env.freeContainerAddress(s.ip)
		env.manageContainerPorts(s.ip.String(), s.portmapping, ClosePorts)
	}
}

// ConfigureDockerNetwork creates a docker network compatible with the enviornment and returns it
func (env *Environment) ConfigureDockerNetwork(containername string) (string, error) {
	return "", errors.New("not yet implemented")
}

// AttachDockerContainer Attach a Docker container to the bridge and the current network environment
func (env *Environment) AttachDockerContainer(containername string) (net.IP, error) {
	pid, err := tenus.DockerPidByName(containername, "/var/run/docker.sock")
	if err != nil {
		return nil, err
	}
	return env.AttachNetworkToContainer(pid, containername, nil)
}

// AttachNetworkToContainer Attach a Docker container to the bridge and the current network environment
func (env *Environment) AttachNetworkToContainer(pid int, sname string, portmapping map[int]int) (net.IP, error) {

	cleanup := func(vethname string) {
		cmd := exec.Command("ip", "link", "delete", vethname)
		_, _ = cmd.Output()
	}

	bridgeVeth, clientVeth, vethIfce, err := env.createVethsPairAndAttachToBridge()
	if err != nil {
		cleanup(bridgeVeth)
		return nil, err
	}

	// Attach veth2 to the docker container
	if err := vethIfce.SetPeerLinkNsPid(pid); err != nil {
		cleanup(bridgeVeth)
		return nil, err
	}

	//generate a new ip for this container
	ip, err := env.generateAddress()
	if err != nil {
		cleanup(bridgeVeth)
		return nil, err
	}

	// set ip to the container veth
	log.Println("Assigning ip ", ip.String()+env.config.HostBridgeMask, " to container ")
	vethGuestIp, vethGuestIpNet, err := net.ParseCIDR(ip.String() + env.config.HostBridgeMask)
	if err != nil {
		cleanup(bridgeVeth)
		env.freeContainerAddress(ip)
		return nil, err
	}

	if err := vethIfce.SetPeerLinkNetInNs(pid, vethGuestIp, vethGuestIpNet, nil); err != nil {
		cleanup(bridgeVeth)
		env.freeContainerAddress(ip)
		return nil, err
	}

	//Add traffic route to bridge
	if err = env.setContainerRoutes(pid, clientVeth); err != nil {
		cleanup(bridgeVeth)
		env.freeContainerAddress(ip)
		return nil, err
	}

	if err = env.Update(); err != nil {
		cleanup(bridgeVeth)
		env.freeContainerAddress(ip)
		return nil, err
	}

	if err = env.setVethFirewallRules(bridgeVeth); err != nil {
		env.freeContainerAddress(ip)
		cleanup(bridgeVeth)
		return nil, err
	}

	if err = env.manageContainerPorts(ip.String(), portmapping, OpenPorts); err != nil {
		debug.PrintStack()
		env.freeContainerAddress(ip)
		cleanup(bridgeVeth)
		return nil, err
	}

	env.deployedServices[sname] = service{
		ip:          ip,
		sname:       sname,
		portmapping: portmapping,
	}
	return ip, nil
}

func (env *Environment) increaseVethsMtu(veth1 string, veth2 string) error {
	log.Println("Changing Veth1's MTU")
	cmd := exec.Command("ip", "link", "set", "dev", veth1, "mtu", env.mtusize)
	_, err := cmd.Output()
	if err != nil {
		return err
	}
	log.Println("Changing Veth2's MTU")
	cmd = exec.Command("ip", "link", "set", "dev", veth2, "mtu", env.mtusize)
	_, err = cmd.Output()
	if err != nil {
		return err
	}
	return nil
}

//create veth pair and connect one to the host bridge
//returns: bridgeVeth name, free Veth name, Vether interface to the veth pair and eventually an error
func (env *Environment) createVethsPairAndAttachToBridge() (string, string, tenus.Vether, error) {
	// Retrieve current bridge
	br, err := tenus.BridgeFromName(env.config.HostBridgeName)
	if err != nil {
		return "", "", nil, err
	}

	veth1name := "veth" + "00" + strconv.Itoa(env.nextVethNumber)
	veth2name := "veth" + "01" + strconv.Itoa(env.nextVethNumber)
	log.Println("creating veth pair: " + veth1name + "@" + veth2name)

	veth, err := tenus.NewVethPairWithOptions(veth1name, tenus.VethOptions{PeerName: veth2name})
	if err != nil {
		env.nextVethNumber = env.nextVethNumber + 1
		return "", "", nil, err
	}

	//Increasing MTUs
	err = env.increaseVethsMtu(veth1name, veth2name)
	if err != nil {
		return "", "", nil, err
	}

	// add veth1 to the bridge
	myveth01, err := net.InterfaceByName(veth1name)
	if err != nil {
		return "", "", nil, err
	}

	if err = br.AddSlaveIfc(myveth01); err != nil {
		return "", "", nil, err
	}

	if err = veth.SetLinkUp(); err != nil {
		return "", "", nil, err
	}
	return veth1name, veth2name, veth, nil
}

// sets the FPRWARD firewall rules for the bridge veth
func (env *Environment) setVethFirewallRules(bridgeVethName string) error {
	// iptables -A FORWARD -o goProxyBridge -i veth -j ACCEPT
	cmd := exec.Command("iptables", "-A", "FORWARD", "-o", env.config.HostBridgeName, "-i", bridgeVethName, "-j", "ACCEPT")
	_, err := cmd.Output()
	if err != nil {
		return err
	}
	cmd = exec.Command("iptables", "-A", "FORWARD", "-i", env.config.HostBridgeName, "-o", bridgeVethName, "-j", "ACCEPT")
	_, err = cmd.Output()
	if err != nil {
		return err
	}
	return nil
}

// add routes inside the container namespace to forward the traffic using the bridge
func (env *Environment) setContainerRoutes(containerPid int, containerVethName string) error {
	//Add route to bridge
	//sudo nsenter -n -t 5565 ip route add 0.0.0.0/0 via 172.18.8.193 dev veth013
	cmd := exec.Command("nsenter", "-n", "-t", strconv.Itoa(containerPid), "ip", "route", "add", "0.0.0.0/0", "via", env.config.HostBridgeIP, "dev", containerVethName)
	_, err := cmd.Output()
	if err != nil {
		log.Println("Impossible to setup route inside the netns")
		return err
	}
	return nil
}

// CreateNetworkNamespaceNewIp creates a new namespace and link it to the host bridge, the ip is going to be generated from the default space
func (env *Environment) CreateNetworkNamespaceNewIp(netname string) (string, error) {
	//generate a new ip for this container
	ip, _ := env.generateAddress()
	return env.CreateNetworkNamespace(netname, ip)
}

// CreateNetworkNamespace creates a new namespace and link it to the host bridge
// netname: short name representative of the namespace, better if a short unique name of the service, max 10 char
func (env *Environment) CreateNetworkNamespace(netname string, ip net.IP) (string, error) {
	//check if appName is valid
	for _, e := range env.nameSpaces {
		if e == netname {
			return "", errors.New(NamespaceAlreadyDeclared)
		}
	}

	//create namespace
	log.Println("creating namespace: " + netname)
	cmd := exec.Command("ip", "netns", "add", netname)
	_, err := cmd.Output()
	if err != nil {
		return "", err
	}

	//create veth pair to connect the namespace to the host bridge
	veth1name := "veth" + "00" + strconv.Itoa(env.nextVethNumber)
	veth2name := "veth" + "01" + strconv.Itoa(env.nextVethNumber)
	log.Println("creating veth pair: " + veth1name + "@" + veth2name)

	cleanup := func() {
		cmd := exec.Command("ip", "link", "delete", veth1name)
		_, _ = cmd.Output()
		cmd = exec.Command("ip", "netns", "delete", netname)
		_, _ = cmd.Output()
	}

	cmd = exec.Command("ip", "link", "add", veth1name, "type", "veth", "peer", "name", veth2name)
	_, err = cmd.Output()
	if err != nil {
		cleanup()
		return "", err
	}

	//Increasing MTUs
	log.Println("Changing Veth1's MTU")
	cmd = exec.Command("ip", "link", "set", "dev", veth1name, "mtu", env.mtusize)
	_, err = cmd.Output()
	if err != nil {
		cleanup()
		return "", err
	}
	log.Println("Changing Veth2's MTU")
	cmd = exec.Command("ip", "link", "set", "dev", veth2name, "mtu", env.mtusize)
	_, err = cmd.Output()
	if err != nil {
		cleanup()
		return "", err
	}

	//attach veth2 to namespace
	log.Println("attaching " + veth2name + " to namespace " + netname)
	cmd = exec.Command("ip", "link", "set", veth2name, "netns", netname)
	_, err = cmd.Output()
	if err != nil {
		cleanup()
		return "", err
	}

	//assign ip to the namespace veth
	log.Println("assigning ip " + ip.String() + env.config.HostBridgeMask + " to " + veth2name)
	cmd = exec.Command("ip", "netns", "exec", netname, "ip", "addr", "add",
		ip.String()+env.config.HostBridgeMask, "dev", veth2name)
	//cmd = exec.Command("ip", "a", "add", ip.String(), "dev", veth2name)
	_, err = cmd.Output()
	if err != nil {
		cleanup()
		return "", err
	}

	//bring ns lo up
	log.Println("bringing lo up")
	cmd = exec.Command("ip", "netns", "exec", netname, "ip", "link", "set", "lo", "up")
	_, err = cmd.Output()
	if err != nil {
		cleanup()
		return "", err
	}

	//bring veth2 up
	log.Println("bringing " + veth2name + " up")
	cmd = exec.Command("ip", "netns", "exec", netname, "ip", "link", "set", veth2name, "up")
	_, err = cmd.Output()
	if err != nil {
		cleanup()
		return "", err
	}

	//attach veth1 to the bridge
	log.Println("attaching " + veth1name + " to host bridge")
	cmd = exec.Command("ip", "link", "set", veth1name, "master", env.config.HostBridgeName)
	_, err = cmd.Output()
	if err != nil {
		cleanup()
		return "", err
	}

	//bring veth1 up
	log.Println("bringing " + veth1name + " up")
	cmd = exec.Command("ip", "link", "set", veth1name, "up")
	_, err = cmd.Output()
	if err != nil {
		cleanup()
		return "", err
	}

	//add rules on netname namespace for routing through the veth
	log.Println("adding default routing rule inside " + netname)
	cmd = exec.Command("ip", "netns", "exec", netname, "ip", "route", "add", "default", "via", env.config.HostBridgeIP, "dev", veth2name)
	_, err = cmd.Output()
	if err != nil {
		cleanup()
		return "", err
	}
	//add rules on default namespace for routing to the new namespace
	log.Println("adding routing rule for default namespace to " + netname)
	cmd = exec.Command("ip", "route", "add", ip.String(), "via", env.config.HostBridgeIP)
	_, err = cmd.Output()
	if err != nil {
		cleanup()
		return "", err
	}

	//disable reverse path filtering
	log.Println("disabling reverse path filtering")
	cmd = exec.Command("echo", "0", ">", "/proc/sys/net/ipv4/conf/all/rp_filter")
	_, err = cmd.Output()
	if err != nil {
		log.Fatal(err.Error())
	}

	//add IP masquerade
	log.Println("add NAT ip MASQUERADING for the bridge")
	cmd = exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", ip.String()+env.config.HostBridgeMask, "-o", env.config.ConnectedInternetInterface, "-j", "MASQUERADE")
	_, err = cmd.Output()
	if err != nil {
		cleanup()
		return "", err
	}

	//add NAT packet forwarding rules
	log.Println("add NAT packet forwarding rules for " + netname)
	cmd = exec.Command("iptables", "-A", "FORWARD", "-o", env.config.ConnectedInternetInterface, "-i", veth1name, "-j", "ACCEPT")
	_, err = cmd.Output()
	if err != nil {
		cleanup()
		return "", err
	}
	cmd = exec.Command("iptables", "-A", "FORWARD", "-i", env.config.ConnectedInternetInterface, "-o", veth1name, "-j", "ACCEPT")
	_, err = cmd.Output()
	if err != nil {
		cleanup()
		return "", err
	}

	err = env.Update()
	if err != nil {
		cleanup()
		return "", err
	}

	return netname, nil
}

func (env *Environment) Destroy() {
	for _, ns := range env.nameSpaces {
		cmd := exec.Command("ip", "netns", "delete", ns)
		_, _ = cmd.Output()
	}
	cmd := exec.Command("ip", "link", "delete", env.config.HostBridgeName)
	_, _ = cmd.Output()
}

// Update update the env class with the current state of the machine. This method must be run always at boot
// updates current declared interfaces and namespaces
func (env *Environment) Update() error {

	// fetch current declared Namespaces
	netns := exec.Command("ip", "netns", "list")
	netnslines, err := netns.Output()
	if err != nil {
		return err
	}
	env.nameSpaces = NetworkNamespacesList(string(netnslines))

	// fetch current declared Veth pairs for default network namespace
	defaultNamespaceVeths, err := env.fetchNetworkVethLinkList("")
	if err != nil {
		return err
	}
	env.networkInterfaces = defaultNamespaceVeths

	nextVeth := 0

	// update next veth number
	for _, iface := range env.networkInterfaces {
		//if is one of the veths declared by us
		if strings.Contains(iface.veth0, "veth00") {
			// assign the next number bigger than the current declared veth
			vethnum, err := strconv.Atoi(strings.Replace(iface.veth0, "veth00", "", -1))
			if err == nil {
				if vethnum >= nextVeth {
					nextVeth = vethnum + 1
				}
			}
		}
	}
	env.nextVethNumber = nextVeth

	return nil
}

//given a namespace returns the veth delcard inside that namespace, empty string is the default namespace
func (env *Environment) fetchNetworkVethLinkList(namespace string) ([]networkInterface, error) {
	var linklines []byte
	var err error
	if namespace == "" {
		link := exec.Command("ip", "link", "list")
		linklines, err = link.Output()
	} else {
		link := exec.Command("ip", "netns", "exec", namespace, "ip", "link", "list")
		linklines, err = link.Output()
	}
	if err != nil {
		return nil, err
	}
	result := NetworkVethLinkList(string(linklines))

	for i := range result {
		elem := result[i]
		elem.namespace = namespace
		result[i] = elem
	}

	return result, nil
}

// CreateHostBridge create host bridge if it has not been created yet, return the current host bridge name or the newly created one
func (env *Environment) CreateHostBridge() (string, error) {
	//check current declared bridges
	bridgecmd := exec.Command("ip", "link", "list", "type", "bridge")
	bridgelines, err := bridgecmd.Output()
	if err != nil {
		return "", err
	}
	currentDeclaredBridges := extractNetInterfaceName(string(bridgelines))

	//is HostBridgeName already created?
	created := false
	for _, name := range currentDeclaredBridges {
		if name == env.config.HostBridgeName {
			created = true
		}
	}

	//if HostBridgeName exists already then return the name
	if created {
		return env.config.HostBridgeName, nil
	}

	//otherwise create it
	createbridgeCmd := exec.Command("ip", "link", "add", "name", env.config.HostBridgeName, "mtu", env.mtusize, "type", "bridge")
	_, err = createbridgeCmd.Output()
	if err != nil {
		return "", err
	}

	//assign ip to the bridge
	bridgeIpCmd := exec.Command("ip", "a", "add",
		env.config.HostBridgeIP+env.config.HostBridgeMask, "dev", env.config.HostBridgeName)
	_, err = bridgeIpCmd.Output()
	if err != nil {
		return "", err
	}

	//bring the bridge up
	bridgeUpCmd := exec.Command("ip", "link", "set", "dev", env.config.HostBridgeName, "up")
	_, err = bridgeUpCmd.Output()
	if err != nil {
		return "", err
	}

	//add iptables rule for forwarding packets
	iptablesCmd := exec.Command("iptables", "-A", "FORWARD", "-i", env.config.HostBridgeName, "-o",
		env.config.HostBridgeName, "-j", "ACCEPT")
	_, err = iptablesCmd.Output()
	if err != nil {
		return "", err
	}

	return env.config.HostBridgeName, nil
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

func (env *Environment) manageContainerPorts(localContainerAddress string, portmapping map[int]int, operation PortOperation) error {
	if portmapping != nil {
		for hostport, containerport := range portmapping {
			sport := strconv.Itoa(hostport)
			destination := fmt.Sprintf("%s:%d", localContainerAddress, containerport)
			openPortCmd := exec.Command("iptables", "-t", "nat", string(operation), "PREROUTING", "-p", "tcp", "--dport", sport, "-j", "DNAT", "--to-destination", destination)
			status, err := openPortCmd.Output()
			if err != nil {
				log.Printf("ERROR: %s\n", string(status))
				return err
			}
			log.Printf("Changed port %s status toward destination %s\n", sport, destination)
		}
	}
	return nil
}

//Given an ipv4, gives the next IP
func nextIP(ip net.IP, inc uint) net.IP {
	i := ip.To4()
	v := uint(i[0])<<24 + uint(i[1])<<16 + uint(i[2])<<8 + uint(i[3])
	v += inc
	v3 := byte(v & 0xFF)
	v2 := byte((v >> 8) & 0xFF)
	v1 := byte((v >> 16) & 0xFF)
	v0 := byte((v >> 24) & 0xFF)
	return net.IPv4(v0, v1, v2, v3)
}
