package proxy

import (
	"NetManager/logger"
	"NetManager/resolver"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"os/exec"

	"golang.zx2c4.com/wireguard/tun"
)

// create a new Tunnel with the configuration from the custom local file. localIP is the
// host address to source tunnel traffic from; discovering it is host-specific and left to the caller.
func New(localIP netip.Addr) *Tunnel {
	// load netcfg.json
	cfg, err := os.Open("/etc/netmanager/tuncfg.json")
	if err != nil {
		logger.ErrorLogger().Println(err)
	} else {
		defer cfg.Close()
	}

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
	return NewCustom(defaultconfig, localIP)
}

// create a new Tunnel with a custom configuration
func NewCustom(configuration Configuration, localIP netip.Addr) *Tunnel {
	tunnel := Tunnel{
		connectionBuffer: make(map[netip.AddrPort]*tunnelConn),
		mtu:              configuration.Mtusize,
	}

	// parse configuration file
	tunconfig := configuration
	tunnel.HostTUNDeviceName = tunconfig.HostTUNDeviceName
	tunnel.TunnelPort = tunconfig.TunnelPort
	tunnel.tunNetIP = tunconfig.TunNetIP
	tunnel.tunNetIPv6 = tunconfig.TunNetIPv6

	v4Prefix := maskedPrefix(tunconfig.ProxySubnetwork, maskBits(tunconfig.ProxySubnetworkMask))
	v6Prefix := maskedPrefix(tunconfig.ProxySubnetworkIPv6, tunconfig.ProxySubnetworkIPv6Prefix)
	tunnel.dp = NewDatapath(nil, localIP.Unmap(), v4Prefix, v6Prefix, &tunnel)

	// create the TUN device
	tunnel.createTun()

	tunnel.startConnectionEviction()

	logger.InfoLogger().Printf("Created ProxyTun device: %s\n", tunnel.tun.Name())
	logger.InfoLogger().Printf("Local Ip detected: %s\n", tunnel.dp.localIP.String())

	return &tunnel
}

// maskBits converts a dotted-quad netmask from the config file into a prefix
// length. The config has always expressed the IPv4 proxy subnetwork that way,
// while the IPv6 one is already a prefix length.
func maskBits(dottedMask string) int {
	mask := net.IPMask(net.ParseIP(dottedMask).To4())
	ones, bits := mask.Size()
	if bits == 0 {
		log.Fatalf("Invalid proxy subnetwork mask: %s", dottedMask)
	}
	return ones
}

// maskedPrefix builds the netip.Prefix the datapath tests destination
// addresses against. Masked() normalizes a prefix whose address carries bits
// below the prefix length, which Contains would otherwise reject outright.
func maskedPrefix(address string, bits int) netip.Prefix {
	addr, err := netip.ParseAddr(address)
	if err != nil {
		log.Fatalf("Unable to parse the proxy subnetwork %q: %s", address, err)
	}
	prefix, err := addr.Unmap().Prefix(bits)
	if err != nil {
		log.Fatalf("Invalid proxy subnetwork %s/%d: %s", address, bits, err)
	}
	return prefix
}

func (t *Tunnel) SetResolver(r resolver.Resolver) {
	t.dp.SetResolver(r)
}

// start listening for packets in the TUN Proxy device
func (t *Tunnel) Listen() {
	logger.InfoLogger().Println("Starting proxy listening mode")
	go t.outgoingLoop()
	go t.ingoingLoop()
}

// logTunEvents drains dev's event channel until the device closes it.
func logTunEvents(dev tun.Device) {
	// the device emits EventMTUUpdate for every link message on the interface,
	// whether or not the MTU moved
	lastMTU := 0
	for event := range dev.Events() {
		switch event {
		case tun.EventUp:
			logger.InfoLogger().Println("TUN device is up")
		case tun.EventDown:
			logger.InfoLogger().Println("TUN device is down")
		case tun.EventMTUUpdate:
			mtu, err := dev.MTU()
			if err != nil {
				logger.ErrorLogger().Println("TUN MTU changed, unable to read it:", err)
				continue
			}
			if mtu != lastMTU {
				logger.InfoLogger().Println("TUN MTU is", mtu)
				lastMTU = mtu
			}
		default:
			logger.InfoLogger().Println("Unknown TUN device event:", event)
		}
	}
}

// create an instance of the proxy TUN device and setup the environment
func (t *Tunnel) createTun() {
	// CreateTUN sets the MTU too, so there's no separate "ip link set mtu" step.
	dev, err := tun.CreateTUN(t.HostTUNDeviceName, t.mtu)
	if err != nil {
		log.Fatalf("Unable to create new TUN/TAP interface: %s", err)
	}
	// Has to start right away: the device's status goroutines do blocking
	// sends on a small buffered channel and hang once it fills up.
	go logTunEvents(dev)

	name, err := dev.Name()
	if err != nil {
		log.Fatalf("Unable to read TUN/TAP interface name: %s", err)
	}

	logger.InfoLogger().Println("Bringing tun up with addr " + t.tunNetIP + "/12")
	cmd := exec.Command("ip", "addr", "add", t.tunNetIP+"/12", "dev", name)
	logger.InfoLogger().Println()
	err = cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	logger.InfoLogger().Println("Bringing tun up with IPv6 addr " + t.tunNetIPv6 + "/7")
	cmd = exec.Command("ip", "addr", "add", t.tunNetIPv6+"/7", "dev", name)
	err = cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	cmd = exec.Command("ip", "link", "set", "dev", name, "up")
	err = cmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	//disabling reverse path filtering
	logger.InfoLogger().Println("Disabling tun dev reverse path filtering")
	cmd = exec.Command("echo", "0", ">", "/proc/sys/net/ipv4/conf/"+name+"/rp_filter")
	err = cmd.Run()
	if err != nil {
		log.Printf("Error disabling tun dev reverse path filtering: %s ", err.Error())
	}

	//Add network routing rule, Done by default by the system
	logger.InfoLogger().Printf("adding routing rule for %s to %s\n", t.dp.ProxyIPv4Prefix.String(), name)
	cmd = exec.Command("ip", "route", "add", "10.30.0.0/12", "dev", name)
	_, _ = cmd.Output()

	//Add network routing rule, Done by default by the system
	logger.InfoLogger().Printf("adding routing rule for %s to %s\n", t.dp.ProxyIPv6Prefix.String(), name)
	cmd = exec.Command("ip", "route", "add", t.dp.ProxyIPv6Prefix.String(), "dev", name)
	_, _ = cmd.Output()

	//add firewalls rules
	logger.InfoLogger().Println("adding firewall rule " + name)
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
	lstnAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%v", t.TunnelPort))
	if nil != err {
		log.Fatal("Unable to get UDP socket:", err)
	}
	lstnConn, err := net.ListenUDP("udp", lstnAddr)
	if nil != err {
		log.Fatal("Unable to listen on UDP socket:", err)
	}
	err = lstnConn.SetReadBuffer(socketBufferSize)
	if nil != err {
		// Not fatal: the socket still works with whatever the kernel's
		// default/clamped size is, just more prone to drops under bursts.
		logger.ErrorLogger().Println("Unable to grow UDP read buffer:", err)
	}

	t.HostTUNDeviceName = name
	t.tun = newWgTunDevice(dev)
	t.sock = newUDPTunnelSocket(lstnConn)
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
