package proxy

import (
	"NetManager/env"
	"NetManager/logger"
	"NetManager/network"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"

	"github.com/songgao/water"
)

// create a  new GoProxyTunnel with the configuration from the custom local file
func New() GoProxyTunnel {
	// load netcfg.json
	cfg, err := os.Open("/etc/netmanager/tuncfg.json")
	if err != nil {
		logger.ErrorLogger().Println(err)
	}
	defer cfg.Close()

	defaultconfig := Configuration{
		HostTUNDeviceName:         "goProxyTun",
		TunNetIP:                  "10.19.1.254",
		ProxySubnetwork:           "10.30.0.0",
		ProxySubnetworkMask:       "255.255.0.0",
		TunnelPort:                50103,
		Mtusize:                   1450,
		TunNetIPv6:                "fcef::dead:beef",
		ProxySubnetworkIPv6:       "fc00::",
		ProxySubnetworkIPv6Prefix: 7,
	}

	jsonparser := json.NewDecoder(cfg)
	if err = jsonparser.Decode(&defaultconfig); err != nil {
		logger.ErrorLogger().Println("error parsing tuncfg.json", err)
	}

	logger.InfoLogger().Printf("Utilizing config: %v", defaultconfig)
	return NewCustom(defaultconfig)
}

// create a  new GoProxyTunnel with a custom configuration
func NewCustom(configuration Configuration) GoProxyTunnel {
	proxy := GoProxyTunnel{
		isListening:      false,
		errorChannel:     make(chan error),
		finishChannel:    make(chan bool),
		stopChannel:      make(chan bool),
		connectionBuffer: make(map[string]*net.UDPConn),
		proxycache:       NewProxyCache(),
		udpwrite:         sync.RWMutex{},
		tunwrite:         sync.RWMutex{},
		incomingChannel:  make(chan incomingMessage, 1000),
		outgoingChannel:  make(chan outgoingMessage, 1000),
		mtusize:          strconv.Itoa(configuration.Mtusize),
		randseed:         rand.New(rand.NewSource(42)),
	}

	// parse configuration file
	tunconfig := configuration
	proxy.HostTUNDeviceName = tunconfig.HostTUNDeviceName
	proxy.ProxyIpSubnetwork = net.IPNet{
		IP:   net.ParseIP(tunconfig.ProxySubnetwork),
		Mask: net.IPMask(net.ParseIP(tunconfig.ProxySubnetworkMask).To4()),
	}
	proxy.TunnelPort = tunconfig.TunnelPort
	proxy.tunNetIP = tunconfig.TunNetIP

	proxy.ProxyIPv6Subnetwork = net.IPNet{
		IP:   net.ParseIP(tunconfig.ProxySubnetworkIPv6),
		Mask: net.CIDRMask(tunconfig.ProxySubnetworkIPv6Prefix, 128),
	}
	proxy.tunNetIPv6 = tunconfig.TunNetIPv6
	// create the TUN device
	proxy.createTun()

	// set local ip
	ipstring, _ := network.GetLocalIPandIface()
	proxy.localIP = net.ParseIP(ipstring)

	logger.InfoLogger().Printf("Created ProxyTun device: %s\n", proxy.ifce.Name())
	logger.InfoLogger().Printf("Local Ip detected: %s\n", proxy.localIP.String())

	return proxy
}

func (proxy *GoProxyTunnel) SetEnvironment(env env.EnvironmentManager) {
	proxy.environment = env
}

func (proxy *GoProxyTunnel) IsListening() bool {
	return proxy.isListening
}

// start listening for packets in the TUN Proxy device
func (proxy *GoProxyTunnel) Listen() {
	if !proxy.isListening {
		logger.InfoLogger().Println("Starting proxy listening mode")
		go proxy.tunOutgoingListen()
		go proxy.tunIngoingListen()
	}
}

// create an instance of the proxy TUN device and setup the environment
func (proxy *GoProxyTunnel) createTun() {
	//create tun device
	config := water.Config{
		DeviceType: water.TUN,
	}
	config.Name = proxy.HostTUNDeviceName
	ifce, err := water.New(config)
	if err != nil {
		log.Fatal(err)
	}

	logger.InfoLogger().Println("Bringing tun up with addr " + proxy.tunNetIP + "/12")
	cmd := exec.Command("ip", "addr", "add", proxy.tunNetIP+"/12", "dev", ifce.Name())
	logger.InfoLogger().Println()
	err = cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	logger.InfoLogger().Println("Bringing tun up with IPv6 addr " + proxy.tunNetIPv6 + "/7")
	cmd = exec.Command("ip", "addr", "add", proxy.tunNetIPv6+"/7", "dev", ifce.Name())
	err = cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	cmd = exec.Command("ip", "link", "set", "dev", ifce.Name(), "up")
	err = cmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	//disabling reverse path filtering
	logger.InfoLogger().Println("Disabling tun dev reverse path filtering")
	cmd = exec.Command("echo", "0", ">", "/proc/sys/net/ipv4/conf/"+ifce.Name()+"/rp_filter")
	err = cmd.Run()
	if err != nil {
		log.Printf("Error disabling tun dev reverse path filtering: %s ", err.Error())
	}

	//Increasing the MTU on the TUN dev
	logger.InfoLogger().Println("Changing TUN's MTU")
	cmd = exec.Command("ip", "link", "set", "dev", ifce.Name(), "mtu", proxy.mtusize)
	err = cmd.Run()
	if err != nil {
		log.Fatal(err.Error())
	}

	//Add network routing rule, Done by default by the system
	logger.InfoLogger().Printf("adding routing rule for %s to %s\n", proxy.ProxyIpSubnetwork.String(), ifce.Name())
	cmd = exec.Command("ip", "route", "add", "10.30.0.0/12", "dev", ifce.Name())
	_, _ = cmd.Output()

	//Add network routing rule, Done by default by the system
	logger.InfoLogger().Printf("adding routing rule for %s to %s\n", proxy.ProxyIPv6Subnetwork.IP.String(), ifce.Name())
	cmd = exec.Command("ip", "route", "add", proxy.ProxyIPv6Subnetwork.IP.String()+proxy.ProxyIPv6Subnetwork.Mask.String(), "dev", ifce.Name())
	_, _ = cmd.Output()

	//add firewalls rules
	logger.InfoLogger().Println("adding firewall rule " + ifce.Name())
	cmd = exec.Command("iptables", "-A", "INPUT", "-i", "tun0", "-m", "state",
		"--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	err = cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	// IPv6
	cmd = exec.Command("ip6tables", "-A", "INPUT", "-i", "tun0", "-m", "state",
		"--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	err = cmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	// listen to local socket
	lstnAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%v", proxy.TunnelPort))
	if nil != err {
		log.Fatal("Unable to get UDP socket:", err)
	}
	lstnConn, err := net.ListenUDP("udp", lstnAddr)
	if nil != err {
		log.Fatal("Unable to listen on UDP socket:", err)
	}
	err = lstnConn.SetReadBuffer(BUFFER_SIZE)
	if nil != err {
		log.Fatal("Unable to set Read Buffer:", err)
	}

	proxy.HostTUNDeviceName = ifce.Name()
	proxy.ifce = ifce
	proxy.listenConnection = lstnConn
}

// Configuration implements Stringer interface
func (c *Configuration) String() string {
	return fmt.Sprintf(
		"HostTUNDeviceName: %s\n"+
			"TunnelIP: %s\n"+
			"ProxySubnetwork: %s\n"+
			"ProxySubnetworkMask: %s\n"+
			"TunnelPort: %d\n"+
			"MTUSize: %d\n"+
			"TunNetIPv6: %s\n"+
			"ProxySubnetworkIPv6: %s\n"+
			"ProxySubnetworkIPv6Prefix: %d\n",
		c.HostTUNDeviceName,
		c.TunNetIP,
		c.ProxySubnetwork,
		c.ProxySubnetworkMask,
		c.TunnelPort,
		c.Mtusize,
		c.TunNetIPv6,
		c.ProxySubnetworkIPv6,
		c.ProxySubnetworkIPv6Prefix,
	)
}
