package proxy

import (
	"NetManager/TableEntryCache"
	"NetManager/env"
	"NetManager/logger"
	"NetManager/network"
	"errors"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/ip4defrag"
	"github.com/google/gopacket/layers"
	"github.com/songgao/water"
	"log"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

// Ipv4 defragger
var defragger = ip4defrag.NewIPv4Defragmenter()

// const
var BUFFER_SIZE = 64 * 1024

// Config
type Configuration struct {
	HostTUNDeviceName   string
	ProxySubnetwork     string
	ProxySubnetworkMask string
	TunNetIP            string
	TunnelPort          int
	Mtusize             string
}

type GoProxyTunnel struct {
	stopChannel       chan bool
	connectionBuffer  map[string]*net.UDPConn
	finishChannel     chan bool
	errorChannel      chan error
	tunNetIP          string
	ifce              *water.Interface
	isListening       bool
	ProxyIpSubnetwork net.IPNet
	HostTUNDeviceName string
	TunnelPort        int
	listenConnection  *net.UDPConn
	bufferPort        int
	environment       env.EnvironmentManager
	proxycache        ProxyCache
	localIP           net.IP
	udpwrite          sync.RWMutex
	tunwrite          sync.RWMutex
	incomingChannel   chan incomingMessage
	outgoingChannel   chan outgoingMessage
	mtusize           string
	randseed          *rand.Rand
}

// incoming message from UDP channel
type incomingMessage struct {
	from    net.UDPAddr
	content *[]byte
}

// outgoing message from bridge
type outgoingMessage struct {
	content *[]byte
}

// create a  new GoProxyTunnel with the configuration from the custom local file
func New() GoProxyTunnel {
	port, err := strconv.Atoi(os.Getenv("PUBLIC_WORKER_PORT"))
	if err != nil {
		logger.InfoLogger().Printf("Default to tunport 50103")
		port = 50103
	}
	mtusize := os.Getenv("TUN_MTU_SIZE")
	if len(mtusize) == 0 {
		logger.InfoLogger().Printf("Default to mtusize 1450")
		mtusize = "1450"
	}
	proxySubnetworkMask := os.Getenv("PROXY_SUBNETWORK_MASK")
	if len(proxySubnetworkMask) == 0 {
		logger.InfoLogger().Printf("Default proxy subnet mask to 255.255.0.0")
		proxySubnetworkMask = "255.255.0.0"
	}
	proxySubnetwork := os.Getenv("PROXY_SUBNETWORK")
	if len(proxySubnetwork) == 0 {
		logger.InfoLogger().Printf("Default proxy subnetwork to 10.30.0.0")
		proxySubnetwork = "10.30.0.0"
	}
	tunNetIP := os.Getenv("TUN_NET_IP")
	if len(tunNetIP) == 0 {
		logger.InfoLogger().Printf("Default to tunNetIP 10.19.1.254")
		tunNetIP = "10.19.1.254"
	}
	tunconfig := Configuration{
		HostTUNDeviceName:   "goProxyTun",
		ProxySubnetwork:     proxySubnetwork,
		ProxySubnetworkMask: proxySubnetworkMask,
		TunNetIP:            tunNetIP,
		TunnelPort:          port,
		Mtusize:             mtusize,
	}
	return NewCustom(tunconfig)
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
		mtusize:          configuration.Mtusize,
		randseed:         rand.New(rand.NewSource(42)),
	}

	//parse confgiuration file
	tunconfig := configuration
	proxy.HostTUNDeviceName = tunconfig.HostTUNDeviceName
	proxy.ProxyIpSubnetwork.IP = net.ParseIP(tunconfig.ProxySubnetwork)
	proxy.ProxyIpSubnetwork.Mask = net.IPMask(net.ParseIP(tunconfig.ProxySubnetworkMask).To4())
	proxy.TunnelPort = tunconfig.TunnelPort
	proxy.tunNetIP = tunconfig.TunNetIP

	//create the TUN device
	proxy.createTun()

	//set local ip
	ipstring, _ := network.GetLocalIPandIface()
	proxy.localIP = net.ParseIP(ipstring)

	logger.InfoLogger().Printf("Created ProxyTun device: %s\n", proxy.ifce.Name())
	logger.InfoLogger().Printf("Local Ip detected: %s\n", proxy.localIP.String())

	return proxy
}

func (proxy *GoProxyTunnel) SetEnvironment(env env.EnvironmentManager) {
	proxy.environment = env
}

// handler function for all outgoing messages that are received by the TUN device
func (proxy *GoProxyTunnel) outgoingMessage() {
	for {
		select {
		case msg := <-proxy.outgoingChannel:

			logger.DebugLogger().Println("outgoingChannelSize: ", len(proxy.outgoingChannel))
			ipv4, tcp, udp := decodePacket(*msg.content)

			if ipv4 != nil {

				logger.DebugLogger().Printf("Outgoing packet from %s\n", ipv4.SrcIP.String())

				// continue only if the packet is udp or tcp, otherwise just drop it
				if tcp != nil || udp != nil {

					//proxyConversion
					newPacket := proxy.outgoingProxy(ipv4, tcp, udp)
					if newPacket == nil {
						//if not proxy conversion available, drop it
						logger.ErrorLogger().Println("Unable to convert the packet")
						continue
					}

					//newTcpLayer := newPacket.Layer(layers.LayerTypeTCP)
					newIpLayer := newPacket.Layer(layers.LayerTypeIPv4)

					//fetch remote address
					dstHost, dstPort := proxy.locateRemoteAddress(newIpLayer.(*layers.IPv4).DstIP)
					logger.DebugLogger().Println("Sending incoming packet to: ", dstHost.String(), ":", dstPort)

					//packetForwarding to tunnel interface
					proxy.forward(dstHost, dstPort, newPacket, 0)
				}
			}
		}
	}
}

// handler function for all ingoing messages that are received by the UDP socket
func (proxy *GoProxyTunnel) ingoingMessage() {
	for {
		select {
		case msg := <-proxy.incomingChannel:
			logger.DebugLogger().Println("ingoingChannelSize: ", len(proxy.incomingChannel))
			ipv4, tcp, udp := decodePacket(*msg.content)
			//from := msg.from

			// proceed only if this is a valid ipv4 packet
			if ipv4 != nil {
				logger.DebugLogger().Printf("Ingoing packet to %s\n", ipv4.DstIP.String())

				// continue only if the packet is udp or tcp, otherwise just drop it
				if tcp != nil || udp != nil {

					// proxyConversion
					newPacket := proxy.ingoingProxy(ipv4, tcp, udp)
					var packetBytes []byte
					if newPacket == nil {
						//no conversion data, forward as is
						packetBytes = *msg.content
					} else {
						packetBytes = packetToByte(newPacket)
					}

					// output to bridge interface
					_, err := proxy.ifce.Write(packetBytes)
					if err != nil {
						logger.ErrorLogger().Println(err)
					}

				}
			}
		}
	}
}

// If packet destination is in the range of proxy.ProxyIpSubnetwork
// then find enable load balancing policy and find out the actual dstIP address
func (proxy *GoProxyTunnel) outgoingProxy(ipv4 *layers.IPv4, tcp *layers.TCP, udp *layers.UDP) gopacket.Packet {
	srcport := -1
	dstport := -1
	if tcp != nil {
		srcport = int(tcp.SrcPort)
		dstport = int(tcp.DstPort)
	}
	if udp != nil {
		srcport = int(udp.SrcPort)
		dstport = int(udp.DstPort)
	}

	//If packet destination is part of the semantic routing subnetwork let the proxy handle it
	semanticRoutingSubnetwork := proxy.ProxyIpSubnetwork.IP.Mask(proxy.ProxyIpSubnetwork.Mask).
		Equal(ipv4.DstIP.Mask(proxy.ProxyIpSubnetwork.Mask))
	if semanticRoutingSubnetwork {

		//Check if the ServiceIP is known
		tableEntryList := proxy.environment.GetTableEntryByServiceIP(ipv4.DstIP)
		if len(tableEntryList) < 1 {
			logger.DebugLogger().Printf("No entries found for this service IP: %s", ipv4.DstIP.String())
			return nil
		}

		//Find the instanceIP of the current service
		instanceIP, err := proxy.convertToInstanceIp(ipv4)
		if err != nil {
			return nil
		}

		//Check proxy proxycache (if any active flow is there already)
		entry, exist := proxy.proxycache.RetrieveByServiceIP(ipv4.SrcIP, instanceIP, srcport, ipv4.DstIP, dstport)
		if !exist || entry.dstport < 1 || !TableEntryCache.IsNamespaceStillValid(entry.dstip, &tableEntryList) {

			//Choose between the table entry according to the ServiceIP algorithm
			//TODO: so far this only uses RR, ServiceIP policies should be implemented here
			tableEntry := tableEntryList[proxy.randseed.Intn(len(tableEntryList))]

			//Update proxycache
			entry = ConversionEntry{
				srcip:         ipv4.SrcIP,
				dstip:         tableEntry.Nsip,
				dstServiceIp:  ipv4.DstIP,
				srcInstanceIp: instanceIP,
				srcport:       srcport,
				dstport:       dstport,
			}
			proxy.proxycache.Add(entry)
		}

		return SerializePacket(entry.dstip, entry.srcInstanceIp, ipv4, tcp, udp)

	}

	return nil
}

func (proxy *GoProxyTunnel) convertToInstanceIp(ipv4 *layers.IPv4) (net.IP, error) {
	instanceTableEntry, instanceexist := proxy.environment.GetTableEntryByNsIP(ipv4.SrcIP)
	instanceIP := net.IP{}
	if instanceexist {
		for _, sip := range instanceTableEntry.ServiceIP {
			if sip.IpType == TableEntryCache.InstanceNumber {
				instanceIP = sip.Address
			}
		}
	} else {
		logger.ErrorLogger().Println("Unable to find instance IP for service: ", ipv4.SrcIP)
		return nil, errors.New(fmt.Sprintf("Unable to find instance IP for service: %s ", ipv4.SrcIP.String()))
	}
	return instanceIP, nil
}

// If packet destination port is proxy.tunnelport then is a packet forwarded by the proxy. The src address must beù
// changed with he original packet destination
func (proxy *GoProxyTunnel) ingoingProxy(ipv4 *layers.IPv4, tcp *layers.TCP, udp *layers.UDP) gopacket.Packet {

	dstport := -1
	srcport := -1

	if tcp != nil {
		dstport = int(tcp.DstPort)
		srcport = int(tcp.SrcPort)
	}
	if udp != nil {
		dstport = int(udp.DstPort)
		srcport = int(udp.SrcPort)
	}

	//Check proxy proxycache for REVERSE entry conversion
	//DstIP -> srcip, DstPort->srcport, srcport -> dstport
	entry, exist := proxy.proxycache.RetrieveByInstanceIp(ipv4.DstIP, dstport, srcport)
	if !exist {
		//No proxy proxycache entry, no translation needed
		return nil
	}

	//Reverse conversion
	return SerializePacket(entry.srcip, entry.dstServiceIp, ipv4, tcp, udp)

}

// start listening for packets in the TUN Proxy device
func (proxy *GoProxyTunnel) Listen() {
	if !proxy.isListening {
		logger.InfoLogger().Println("Starting proxy listening mode")
		go proxy.tunOutgoingListen()
		go proxy.tunIngoingListen()
	}
}

func (proxy *GoProxyTunnel) IsListening() bool {
	return proxy.isListening
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

	//add firewalls rules
	logger.InfoLogger().Println("adding firewall roule " + ifce.Name())
	cmd = exec.Command("iptables", "-A", "INPUT", "-i", "tun0", "-m", "state",
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

// Enable listening to outgoing packets
// if the goroutine must be stopped, send true to the stop channel
// when the channels finish listening a "true" is sent back to the finish channel
// in case of fatal error they are routed back to the err channel
func (proxy *GoProxyTunnel) tunOutgoingListen() {
	readerror := make(chan error)

	//async listener
	go proxy.ifaceread(proxy.ifce, proxy.outgoingChannel, readerror)

	//async handler
	go proxy.outgoingMessage()

	proxy.isListening = true
	logger.InfoLogger().Println("GoProxyTunnel outgoing listening started")
	for {
		select {
		case stopmsg := <-proxy.stopChannel:
			if stopmsg {
				logger.DebugLogger().Println("Outgoing listener received stop message")
				proxy.isListening = false
				proxy.finishChannel <- true
				return
			}
		case errormsg := <-readerror:
			proxy.errorChannel <- errormsg
		}
	}
}

// Enable listening for ingoing packets
// if the goroutine must be stopped, send true to the stop channel
// when the channels finish listening a "true" is sent back to the finish channel
// in case of fatal error they are routed back to the err channel
func (proxy *GoProxyTunnel) tunIngoingListen() {
	readerror := make(chan error)

	//async listener
	go proxy.udpread(proxy.listenConnection, proxy.incomingChannel, readerror)

	//async handler
	go proxy.ingoingMessage()

	proxy.isListening = true
	logger.InfoLogger().Println("GoProxyTunnel ingoing listening started")
	for {
		select {
		case stopmsg := <-proxy.stopChannel:
			if stopmsg {
				logger.DebugLogger().Println("Ingoing listener received stop message")
				_ = proxy.listenConnection.Close()
				proxy.isListening = false
				proxy.finishChannel <- true
				return
			}
		case errormsg := <-readerror:
			proxy.errorChannel <- errormsg
			//go udpread(proxy.listenConnection, readoutput, readerror)
		}
	}
}

// Given a network namespace IP find the machine IP and port for the tunneling
func (proxy *GoProxyTunnel) locateRemoteAddress(nsIP net.IP) (net.IP, int) {

	//if no local cache entry convert namespace IP to host IP via table query
	tableElement, found := proxy.environment.GetTableEntryByNsIP(nsIP)
	if found {
		logger.DebugLogger().Println("Remote NS IP", nsIP.String(), " translated to ", tableElement.Nodeip.String())
		return tableElement.Nodeip, tableElement.Nodeport
	}

	//If nothing found, just drop the packet using an invalid port
	return nsIP, -1

}

// forward message to final destination via UDP tunneling
func (proxy *GoProxyTunnel) forward(dstHost net.IP, dstPort int, packet gopacket.Packet, attemptNumber int) {

	if attemptNumber > 10 {
		return
	}

	packetBytes := packetToByte(packet)

	//If destination host is this machine, forward packet directly to the ingoing traffic method
	if dstHost.Equal(proxy.localIP) {
		logger.InfoLogger().Println("Packet forwarded locally")
		msg := incomingMessage{
			from: net.UDPAddr{
				IP:   proxy.localIP,
				Port: 0,
				Zone: "",
			},
			content: &packetBytes,
		}
		proxy.incomingChannel <- msg
		return
	}

	//Check udp channel buffer to avoid creating a new channel
	proxy.udpwrite.Lock()
	hoststring := fmt.Sprintf("%s:%v", dstHost, dstPort)
	con, exist := proxy.connectionBuffer[hoststring]
	proxy.udpwrite.Unlock()
	//TODO: flush connection buffer by time to time
	if !exist {
		logger.DebugLogger().Println("Establishing a new connection to node ", hoststring)
		connection, err := createUDPChannel(hoststring)
		if nil != err {
			return
		}
		_ = connection.SetWriteBuffer(BUFFER_SIZE)
		proxy.udpwrite.Lock()
		proxy.connectionBuffer[hoststring] = connection
		proxy.udpwrite.Unlock()
		con = connection
	}

	//send via UDP channel
	proxy.udpwrite.Lock()
	_, _, err := (*con).WriteMsgUDP(packetBytes, nil, nil)
	proxy.udpwrite.Unlock()
	if err != nil {
		_ = (*con).Close()
		logger.ErrorLogger().Println(err)
		connection, err := createUDPChannel(hoststring)
		if nil != err {
			return
		}
		proxy.udpwrite.Lock()
		proxy.connectionBuffer[hoststring] = connection
		proxy.udpwrite.Unlock()
		//Try again
		attemptNumber++
		proxy.forward(dstHost, dstPort, packet, attemptNumber)
	}
}

func createUDPChannel(hoststring string) (*net.UDPConn, error) {
	raddr, err := net.ResolveUDPAddr("udp", hoststring)
	if err != nil {
		logger.ErrorLogger().Println("Unable to resolve remote addr:", err)
		return nil, err
	}
	connection, err := net.DialUDP("udp", nil, raddr)
	if nil != err {
		logger.ErrorLogger().Println("Unable to connect to remote addr:", err)
		return nil, err
	}
	err = connection.SetWriteBuffer(BUFFER_SIZE)
	if nil != err {
		logger.ErrorLogger().Println("Buffer error:", err)
		return nil, err
	}
	return connection, nil
}

// read output from an interface and wrap the read operation with a channel
// out channel gives back the byte array of the output
// errchannel is the channel where in case of error the error is routed
func (proxy *GoProxyTunnel) ifaceread(ifce *water.Interface, out chan<- outgoingMessage, errchannel chan<- error) {
	buffer := make([]byte, BUFFER_SIZE)
	for true {
		n, err := ifce.Read(buffer)
		if err != nil {
			errchannel <- err
		} else {
			res := make([]byte, n)
			copy(res, buffer[:n])
			logger.DebugLogger().Printf("Outgoing packet ready for decode action \n")
			out <- outgoingMessage{
				content: &res,
			}
		}
	}
}

// read output from an UDP connection and wrap the read operation with a channel
// out channel gives back the byte array of the output
// errchannel is the channel where in case of error the error is routed
func (proxy *GoProxyTunnel) udpread(conn *net.UDPConn, out chan<- incomingMessage, errchannel chan<- error) {
	buffer := make([]byte, BUFFER_SIZE)
	for true {
		packet := buffer
		n, from, err := conn.ReadFromUDP(packet)
		if err != nil {
			errchannel <- err
		} else {
			res := make([]byte, n)
			copy(res, buffer[:n])
			out <- incomingMessage{
				from:    *from,
				content: &res,
			}
		}
	}
}

func packetToByte(packet gopacket.Packet) []byte {
	options := gopacket.SerializeOptions{
		ComputeChecksums: false,
		FixLengths:       true,
	}
	newBuffer := gopacket.NewSerializeBuffer()
	err := gopacket.SerializePacket(newBuffer, options, packet)
	if err != nil {
		logger.ErrorLogger().Println(err)
	}
	return newBuffer.Bytes()
}

// GetName returns the name of the tun interface
func (proxy *GoProxyTunnel) GetName() string {
	return proxy.HostTUNDeviceName
}

// GetErrCh returns the error channel
// this channel sends all the errors of the tun device
func (proxy *GoProxyTunnel) GetErrCh() <-chan error {
	return proxy.errorChannel
}

// GetStopCh returns the errCh
// this channel is used to stop the service. After a shutdown the TUN device stops listening
func (proxy *GoProxyTunnel) GetStopCh() chan<- bool {
	return proxy.stopChannel
}

// GetFinishCh returns the confirmation that the channel stopped listening for packets
func (proxy *GoProxyTunnel) GetFinishCh() <-chan bool {
	return proxy.finishChannel
}

func SerializePacket(dstIp net.IP, srcIp net.IP, ip *layers.IPv4, tcp *layers.TCP, udp *layers.UDP) gopacket.Packet {
	ip.DstIP = dstIp
	ip.SrcIP = srcIp

	if tcp != nil {
		return serializeTcpPacket(tcp, ip)
	} else {
		return serializeUdpPacket(udp, ip)
	}
}

func serializeTcpPacket(tcp *layers.TCP, ip *layers.IPv4) gopacket.Packet {
	err := tcp.SetNetworkLayerForChecksum(ip)
	if err != nil {
		logger.ErrorLogger().Println(err)
	}
	return serializeIpPacket(ip, tcp, gopacket.Payload(tcp.Payload))
}

func serializeUdpPacket(udp *layers.UDP, ip *layers.IPv4) gopacket.Packet {
	err := udp.SetNetworkLayerForChecksum(ip)
	if err != nil {
		logger.ErrorLogger().Println(err)
	}
	return serializeIpPacket(ip, udp, gopacket.Payload(udp.Payload))
}

func serializeIpPacket(ip *layers.IPv4, transportLayer gopacket.SerializableLayer, payload gopacket.SerializableLayer) gopacket.Packet {
	newBuffer := gopacket.NewSerializeBuffer()
	err := ip.SerializeTo(newBuffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true})
	if err != nil {
		logger.ErrorLogger().Println(err)
	}

	buffer := gopacket.NewSerializeBuffer()
	err = gopacket.SerializeLayers(
		buffer,
		gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true},
		ip,
		transportLayer,
		payload,
	)
	if err != nil {
		logger.ErrorLogger().Printf("packet serialization failure %v\n", err)
		return nil
	}

	return gopacket.NewPacket(buffer.Bytes(), layers.LayerTypeIPv4, gopacket.Default)
}

func decodePacket(msg []byte) (*layers.IPv4, *layers.TCP, *layers.UDP) {
	packet := gopacket.NewPacket(msg, layers.LayerTypeIPv4, gopacket.Default)
	ipLayer := packet.NetworkLayer()

	if ipLayer == nil {
		logger.ErrorLogger().Println("ipv4 decode] ")
		return nil, nil, nil
	}

	//defragment if necessary
	ipdefrag, err := defragger.DefragIPv4(ipLayer.(*layers.IPv4))
	if err != nil {
		logger.ErrorLogger().Println(err)
		return nil, nil, nil
	} else if ipdefrag == nil {
		return nil, nil, nil // packet fragment, we don't have whole packet yet.
	}

	pb, ok := packet.(gopacket.PacketBuilder)
	if !ok {
		logger.ErrorLogger().Println("invalid packet builder")
		return nil, nil, nil
	}

	nextDecoder := ipdefrag.NextLayerType()
	err = nextDecoder.Decode(ipdefrag.Payload, pb)
	if err != nil {
		logger.ErrorLogger().Printf("decoder error %v\n", err)
		return nil, nil, nil
	}

	switch ipdefrag.NextLayerType() {
	case layers.LayerTypeUDP:
		udplayer := packet.Layer(layers.LayerTypeUDP)
		return ipdefrag, nil, udplayer.(*layers.UDP)
	case layers.LayerTypeTCP:
		udplayer := packet.Layer(layers.LayerTypeTCP)
		return ipdefrag, udplayer.(*layers.TCP), nil
	default:
		return ipdefrag, nil, nil
	}

}
