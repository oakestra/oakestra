package proxy

import (
	"NetManager/env"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/songgao/water"
	"github.com/tkanos/gonfig"
	"log"
	"math/rand"
	"net"
	"os/exec"
	"runtime"
	"sync"
)

//const
var BUFFER_SIZE = 64 * 1024

// Config
type Configuration struct {
	HostTUNDeviceName   string
	ProxySubnetwork     string
	ProxySubnetworkMask string
	TunNetIP            string
	TunnelPort          int
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
	HostCache         HostCache
	localIP           net.IP
	udpwrite          sync.RWMutex
	tunwrite          sync.RWMutex
	incomingChannel   chan incomingMessage
	outgoingChannel   chan []byte
	readUdpBuffer     []byte
	readTunBuffer     []byte
}

//incoming message from UDP channel
type incomingMessage struct {
	from    net.UDPAddr
	content []byte
}

// create a  new GoProxyTunnel with the configuration from the custom local file
func New() GoProxyTunnel {
	//parse confgiuration file
	tunconfig := Configuration{}
	err := gonfig.GetConf("config/tuncfg.json", &tunconfig)
	if err != nil {
		log.Fatal(err)
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
		HostCache:        NewHostCache(),
		udpwrite:         sync.RWMutex{},
		tunwrite:         sync.RWMutex{},
		incomingChannel:  make(chan incomingMessage),
		outgoingChannel:  make(chan []byte),
		readUdpBuffer:    make([]byte, BUFFER_SIZE),
		readTunBuffer:    make([]byte, BUFFER_SIZE),
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
	ipstring, _ := env.GetLocalIPandIface()
	proxy.localIP = net.ParseIP(ipstring)

	log.Printf("Created ProxyTun device: %s\n", proxy.ifce.Name())

	return proxy
}

func (proxy *GoProxyTunnel) SetEnvironment(env env.EnvironmentManager) {
	proxy.environment = env
}

//handler function for all outgoing messages that are received by the TUN device
func (proxy *GoProxyTunnel) outgoingMessage() {
	for {
		select {
		case msg := <-proxy.outgoingChannel:
			packet := gopacket.NewPacket(msg, layers.LayerTypeIPv4, gopacket.Default)

			//If this is an IP packet
			if ipLayer := packet.Layer(layers.LayerTypeIPv4); ipLayer != nil {

				tcpLayer := packet.Layer(layers.LayerTypeTCP)
				udpLayer := packet.Layer(layers.LayerTypeUDP)

				// continue only if the packet is udp or tcp, otherwise just drop it
				if tcpLayer != nil || udpLayer != nil {

					//ipv4, _ := ipLayer.(*layers.IPv4)
					//fmt.Printf("From src ip %d to dst ip %d\n", ipv4.SrcIP, ipv4.DstIP)

					//proxyConversion
					newPacket := proxy.outgoingProxy(packet)

					//newTcpLayer := newPacket.Layer(layers.LayerTypeTCP)
					newIpLayer := newPacket.Layer(layers.LayerTypeIPv4)

					//fetch remote address
					dstHost, dstPort := proxy.locateRemoteAddress(newIpLayer.(*layers.IPv4).DstIP)
					//log.Println("Sending incoming packet to: ", dstHost.String(), ":", dstPort)

					//packetForwarding
					proxy.forward(dstHost, dstPort, newPacket, 0)
				}
			}
		}
	}
}

//handler function for all ingoing messages that are received by the UDP socket
func (proxy *GoProxyTunnel) ingoingMessage() {
	for {
		select {
		case msg := <-proxy.incomingChannel:
			packet := gopacket.NewPacket(msg.content, layers.LayerTypeIPv4, gopacket.Default)
			//from := msg.from

			//If this is an IP packet
			if ipLayer := packet.Layer(layers.LayerTypeIPv4); ipLayer != nil {

				tcpLayer := packet.Layer(layers.LayerTypeTCP)
				udpLayer := packet.Layer(layers.LayerTypeUDP)

				// continue only if the packet is udp or tcp, otherwise just drop it
				if tcpLayer != nil || udpLayer != nil {

					//if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
					//ipv4, _ := ipLayer.(*layers.IPv4)
					//fmt.Printf("From src ip %d to dst ip %d\n", ipv4.SrcIP, ipv4.DstIP)

					//proxyConversion
					newPacket := proxy.ingoingProxy(packet)

					//cache host connection
					//proxy.HostCache.Add(HostEntry{
					//	srcip: ipv4.SrcIP,
					//	host:  from,
					//})
					//TODO: cache host connection
					//TODO: optimize for paraller traffic

					//send message to TUN
					//proxy.tunwrite.Lock()
					//defer proxy.tunwrite.Unlock()
					_, err := proxy.ifce.Write(packetToByte(newPacket))
					if err != nil {
						fmt.Println("[ERROR]", err)
					}

				}
			}
		}
	}
}

//If packet destination is in the range of proxy.ProxyIpSubnetwork
//then find enable load balancing policy and find out the actual dstIP address
func (proxy *GoProxyTunnel) outgoingProxy(packet gopacket.Packet) gopacket.Packet {
	if ipLayer := packet.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		ipv4, _ := ipLayer.(*layers.IPv4)
		srcport, dstport := -1, -1
		if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
			tcp, _ := tcpLayer.(*layers.TCP)
			srcport = int(tcp.SrcPort)
			dstport = int(tcp.DstPort)
		}
		if udpLayer := packet.Layer(layers.LayerTypeUDP); udpLayer != nil {
			udp, _ := udpLayer.(*layers.UDP)
			srcport = int(udp.SrcPort)
			dstport = int(udp.DstPort)
		}

		//If packet destination is part of the ProxyIP subnetwork Make the proxy handle it
		sameSubnetwork := proxy.ProxyIpSubnetwork.IP.Mask(proxy.ProxyIpSubnetwork.Mask).
			Equal(ipv4.DstIP.Mask(proxy.ProxyIpSubnetwork.Mask))
		if sameSubnetwork {

			//Check proxy proxycache
			entry, exist := proxy.proxycache.RetrieveByServiceIP(ipv4.SrcIP, srcport, ipv4.DstIP, dstport)
			if !exist {
				//If no proxycache entry ask to the environment for a TableQuery
				tableEntryList := proxy.environment.GetTableEntryByServiceIP(ipv4.DstIP)

				//If no table entry available
				if len(tableEntryList) < 1 {
					//discard packet
					return packet
				}

				//Choose between the table entry according to the ServiceIP algorithm
				tableEntry := tableEntryList[rand.Intn(len(tableEntryList))]

				//Find the instanceIP of the current service
				instanceTableEntry, instanceexist := proxy.environment.GetTableEntryByNsIP(ipv4.SrcIP)
				instanceIP := net.IP{}
				if instanceexist {
					for _, sip := range instanceTableEntry.ServiceIP {
						if sip.IpType == env.InstanceNumber {
							instanceIP = sip.Address
						}
					}
				} else {
					log.Println("[Error] Unable to find instance IP for service: ", ipv4.SrcIP)
					return packet
				}

				//TODO smart ServiceIP algorithms

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

			return OutgoingConversion(entry.dstip, entry.srcInstanceIp, packet)

		}

	}
	return packet
}

//If packet destination port is proxy.tunnelport then is a packet forwarded by the proxy. The src address must beù
//changed with he original packet destination
func (proxy *GoProxyTunnel) ingoingProxy(packet gopacket.Packet) gopacket.Packet {
	if ipLayer := packet.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
			ipv4, _ := ipLayer.(*layers.IPv4)
			tcp, _ := tcpLayer.(*layers.TCP)

			//Check proxy proxycache for REVERSE entry conversion
			//DstIP -> srcip, DstPort->srcport, srcport -> dstport
			entry, exist := proxy.proxycache.RetrieveByInstanceIp(ipv4.DstIP, int(tcp.DstPort), int(tcp.SrcPort))
			if !exist {
				//No proxy proxycache entry, no translation needed
				return packet
			}
			//Reverse conversion
			return IngoingConversion(entry.dstServiceIp, entry.srcip, packet)

		}
	}
	return packet
}

// start listening for packets in the TUN Proxy device
func (proxy *GoProxyTunnel) Listen() {
	if !proxy.isListening {
		log.Println("Starting proxy listening mode")
		go proxy.tunOutgoingListen()
		go proxy.tunIngoingListen()
	}
}

//create an instance of the proxy TUN device and setup the environment
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

	log.Println("Bringing tun up with addr " + proxy.tunNetIP + "/12")
	cmd := exec.Command("ip", "addr", "add", proxy.tunNetIP+"/12", "dev", ifce.Name())
	_, err = cmd.Output()
	if err != nil {
		log.Fatal(err)
	}
	cmd = exec.Command("ip", "link", "set", "dev", ifce.Name(), "up")
	_, err = cmd.Output()
	if err != nil {
		log.Fatal(err)
	}

	//disabling reverse path filtering
	log.Println("Disabling tun dev reverse path filtering")
	cmd = exec.Command("echo", "0", ">", "/proc/sys/net/ipv4/conf/"+ifce.Name()+"/rp_filter")
	_, err = cmd.Output()
	if err != nil {
		log.Fatal(err.Error())
	}

	//Increasing the MTU on the TUN dev
	log.Println("Changing TUN's MTU")
	cmd = exec.Command("ip", "link", "set", "dev", ifce.Name(), "mtu", "1472")
	_, err = cmd.Output()
	if err != nil {
		log.Fatal(err.Error())
	}

	//Add network routing rule, Done by default by the system
	log.Println("adding routing rule for 172.30.0.0/12 to " + ifce.Name())
	cmd = exec.Command("ip", "route", "add", "172.30.0.0/12", "dev", ifce.Name())
	_, _ = cmd.Output()

	//add firewalls rules
	log.Println("adding firewall roule " + ifce.Name())
	/*cmd = exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-o", ifce.Name(), "-j", "MASQUERADE")
	_, err = cmd.Output()
	if err != nil {
		log.Fatal(err)
	}*/
	cmd = exec.Command("iptables", "-A", "INPUT", "-i", "tun0", "-m", "state",
		"--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	_, err = cmd.Output()
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
	for i := 0; i < runtime.NumCPU(); i++ {
		log.Println("Started outgoing listener")
		go proxy.ifaceread(proxy.ifce, proxy.outgoingChannel, readerror)
	}
	//async handler
	for i := 0; i < runtime.NumCPU(); i++ {
		log.Println("Started outgoing handler")
		go proxy.outgoingMessage()
	}

	proxy.isListening = true
	log.Println("GoProxyTunnel outgoing listening started")
	for {
		select {
		case stopmsg := <-proxy.stopChannel:
			if stopmsg {
				fmt.Println("Outgoing listener received stop message")
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
	for i := 0; i < runtime.NumCPU(); i++ {
		log.Println("Started ingoing listener")
		go proxy.udpread(proxy.listenConnection, proxy.incomingChannel, readerror)
	}
	//async handler
	for i := 0; i < runtime.NumCPU(); i++ {
		log.Println("Started ingoing handler")
		go proxy.ingoingMessage()
	}

	proxy.isListening = true
	log.Println("GoProxyTunnel ingoing listening started")
	for {
		select {
		case stopmsg := <-proxy.stopChannel:
			if stopmsg {
				fmt.Println("Ingoing listener received stop message")
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

//Given a network namespace IP find the machine IP and port for the tunneling
func (proxy *GoProxyTunnel) locateRemoteAddress(nsIP net.IP) (net.IP, int) {

	//check local Host Cache
	//TODO populate local host cache
	hostentry, exist := proxy.HostCache.Get(nsIP)
	if exist {
		return hostentry.host.IP, hostentry.host.Port
	}

	//if no local cache entry convert namespace IP to host IP via table query
	tableElement, found := proxy.environment.GetTableEntryByNsIP(nsIP)
	if found {
		//log.Println("Remote NS IP", nsIP.String(), " translated to ", tableElement.Nodeip.String())
		return tableElement.Nodeip, tableElement.Nodeport
	}

	//If nothing found, just let the packet to be dropped
	return nsIP, -1

}

//forward message to final destination via UDP tunneling
func (proxy *GoProxyTunnel) forward(dstHost net.IP, dstPort int, packet gopacket.Packet, attemptNumber int) {

	if attemptNumber > 10 {
		return
	}

	//If destination host is this machine, forward packet directly to the ingoing traffic method
	if dstHost.Equal(proxy.localIP) {
		//log.Println("Packet forwarded locally")
		msg := incomingMessage{
			from: net.UDPAddr{
				IP:   nil,
				Port: 0,
				Zone: "",
			},
			content: packet.Data(),
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
		log.Println("Establishing a new connection to node ", hoststring)
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
	_, _, err := (*con).WriteMsgUDP(packetToByte(packet), nil, nil)
	if err != nil {
		_ = (*con).Close()
		log.Println("[ERROR]", err)
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
		log.Println("[ERROR] Unable to resolve remote addr:", err)
		return nil, err
	}
	connection, err := net.DialUDP("udp", nil, raddr)
	if nil != err {
		log.Println("[ERROR] Unable to connect to remote addr:", err)
		return nil, err
	}
	err = connection.SetWriteBuffer(BUFFER_SIZE)
	if nil != err {
		log.Println("[ERROR] Buffer error:", err)
		return nil, err
	}
	return connection, nil
}

// read output from an interface and wrap the read operation with a channel
// out channel gives back the byte array of the output
// errchannel is the channel where in case of error the error is routed
func (proxy *GoProxyTunnel) ifaceread(ifce *water.Interface, out chan<- []byte, errchannel chan<- error) {
	for true {
		packet := proxy.readTunBuffer
		n, err := ifce.Read(packet)
		log.Println("ifaceread Packet size ", n)
		if err != nil {
			errchannel <- err
		}
		out <- packet[:n]
	}
}

// read output from an UDP connection and wrap the read operation with a channel
// out channel gives back the byte array of the output
// errchannel is the channel where in case of error the error is routed
func (proxy *GoProxyTunnel) udpread(conn *net.UDPConn, out chan<- incomingMessage, errchannel chan<- error) {
	for true {
		packet := proxy.readUdpBuffer
		n, from, err := conn.ReadFromUDP(packet)
		log.Println("udp Packet size ", n)
		if err != nil {
			errchannel <- err
		} else {
			out <- incomingMessage{
				from:    *from,
				content: packet[:n],
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
		fmt.Println("[ERROR]", err)
	}
	return newBuffer.Bytes()
}

// returns the name of the tun interface
func (proxy *GoProxyTunnel) GetName() string {
	return proxy.HostTUNDeviceName
}

// returns the error channel
// this channel sends all the errors of the tun device
func (proxy *GoProxyTunnel) GetErrCh() <-chan error {
	return proxy.errorChannel
}

// returns the errCh
// this channel is used to stop the service. After a shutdown the TUN device stops listening
func (proxy *GoProxyTunnel) GetStopCh() chan<- bool {
	return proxy.stopChannel
}

// returns the confirmation that the channel stopped listening for packets
func (proxy *GoProxyTunnel) GetFinishCh() <-chan bool {
	return proxy.finishChannel
}

func OutgoingConversion(dstIp net.IP, srcIp net.IP, packet gopacket.Packet) gopacket.Packet {

	ip := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	ip.DstIP = dstIp
	ip.SrcIP = srcIp

	if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
		return serializeTcpPacket(tcpLayer.(*layers.TCP), ip, packet)
	} else {
		udpLayer := packet.Layer(layers.LayerTypeUDP)
		return serializeUdpPacket(udpLayer.(*layers.UDP), ip, packet)
	}

}

func IngoingConversion(srcIP net.IP, dstIP net.IP, packet gopacket.Packet) gopacket.Packet {
	ip := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	ip.SrcIP = srcIP
	ip.DstIP = dstIP

	if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
		return serializeTcpPacket(tcpLayer.(*layers.TCP), ip, packet)
	} else {
		udpLayer := packet.Layer(layers.LayerTypeUDP)
		return serializeUdpPacket(udpLayer.(*layers.UDP), ip, packet)
	}
}

func serializeTcpPacket(tcp *layers.TCP, ip *layers.IPv4, packet gopacket.Packet) gopacket.Packet {
	err := tcp.SetNetworkLayerForChecksum(ip)
	if err != nil {
		log.Println("[ERROR] ", err)
	}

	newBuffer := gopacket.NewSerializeBuffer()
	err = gopacket.SerializePacket(newBuffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, packet)
	if err != nil {
		log.Println("[ERROR] ", err)
	}

	return gopacket.NewPacket(newBuffer.Bytes(), layers.LayerTypeIPv4, gopacket.Default)
}

func serializeUdpPacket(udp *layers.UDP, ip *layers.IPv4, packet gopacket.Packet) gopacket.Packet {
	err := udp.SetNetworkLayerForChecksum(ip)
	if err != nil {
		log.Println("[ERROR] ", err)
	}

	newBuffer := gopacket.NewSerializeBuffer()
	err = gopacket.SerializePacket(newBuffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, packet)
	if err != nil {
		log.Println("[ERROR] ", err)
	}

	return gopacket.NewPacket(newBuffer.Bytes(), layers.LayerTypeIPv4, gopacket.Default)
}
