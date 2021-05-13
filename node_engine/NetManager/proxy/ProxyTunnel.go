package proxy

import (
	"../env"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/songgao/water"
	"github.com/tkanos/gonfig"
	"log"
	"math/rand"
	"net"
	"os/exec"
)

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
	cache             ProxyCache
	localIP           net.IP
}

// create a  new GoProxyTunnel as a Tun device that listen for packets and forward them to an handler function
func New() GoProxyTunnel {
	proxy := GoProxyTunnel{
		isListening:   false,
		errorChannel:  make(chan error),
		finishChannel: make(chan bool),
		stopChannel:   make(chan bool),
		cache:         NewProxyCache(),
	}

	//parse confgiuration file
	tunconfig := Configuration{}
	err := gonfig.GetConf("config/tuncfg.json", &tunconfig)
	if err != nil {
		log.Fatal(err)
	}
	proxy.HostTUNDeviceName = tunconfig.HostTUNDeviceName
	proxy.ProxyIpSubnetwork.IP = net.ParseIP(tunconfig.ProxySubnetwork)
	proxy.ProxyIpSubnetwork.Mask = net.IPMask(net.ParseIP(tunconfig.ProxySubnetworkMask).To4())
	proxy.TunnelPort = tunconfig.TunnelPort
	proxy.tunNetIP = tunconfig.TunNetIP

	//create the TUN device
	proxy.createTun()

	//set local ip
	proxy.localIP = net.ParseIP(getLocalIP())

	log.Printf("Created ProxyTun device: %s\n", proxy.ifce.Name())

	return proxy
}

func (proxy *GoProxyTunnel) SetEnvironment(env env.EnvironmentManager) {
	proxy.environment = env
}

//handler function for all outgoing messages that are received by the TUN device
func (proxy *GoProxyTunnel) outgoingMessage(packet gopacket.Packet) {
	//If this is an IP packet
	if ipLayer := packet.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer == nil {
			if udpLayer := packet.Layer(layers.LayerTypeUDP); udpLayer == nil {
				return
			}
		}
		fmt.Println("L4 packet received")
		ipv4, _ := ipLayer.(*layers.IPv4)
		fmt.Printf("From src ip %d to dst ip %d\n", ipv4.SrcIP, ipv4.DstIP)

		//proxyConversion
		newPacket := proxy.outgoingProxy(packet)

		//newTcpLayer := newPacket.Layer(layers.LayerTypeTCP)
		newIpLayer := newPacket.Layer(layers.LayerTypeIPv4)

		//fetch remote address
		dstHost, dstPort := proxy.locateRemoteAddress(newIpLayer.(*layers.IPv4).DstIP)
		log.Println("Sending incoming packet to: ", dstHost.String(), ":", dstPort)

		//packetForwarding
		proxy.forward(dstHost, dstPort, newPacket)
	}
}

//handler function for all ingoing messages that are received by the UDP socket
func (proxy *GoProxyTunnel) ingoingMessage(packet gopacket.Packet) {
	//If this is an IP packet
	if ipLayer := packet.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
			fmt.Println("TCP packet received")
			ipv4, _ := ipLayer.(*layers.IPv4)
			fmt.Printf("From src ip %d to dst ip %d\n", ipv4.SrcIP, ipv4.DstIP)
			tcp, _ := tcpLayer.(*layers.TCP)
			fmt.Printf("From src port %d to dst port %d\n", tcp.SrcPort, tcp.DstPort)

			//proxyConversion
			newPacket := proxy.ingoingProxy(packet)

			//send message to TUN
			_, err := proxy.ifce.Write(packetToByte(newPacket))
			if err != nil {
				fmt.Println("[ERROR]", err)
			}
		}
	}
}

//If packet destination is in the range of proxy.ProxyIpSubnetwork
//then find enable load balancing policy and find out the actual dstIP address
func (proxy *GoProxyTunnel) outgoingProxy(packet gopacket.Packet) gopacket.Packet {
	if ipLayer := packet.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
			ipv4, _ := ipLayer.(*layers.IPv4)
			tcp, _ := tcpLayer.(*layers.TCP)
			//If packet destination is part of the ProxyIP subnetwork Make the proxy handle it
			sameSubnetwork := proxy.ProxyIpSubnetwork.IP.Mask(proxy.ProxyIpSubnetwork.Mask).
				Equal(ipv4.DstIP.Mask(proxy.ProxyIpSubnetwork.Mask))
			if sameSubnetwork {
				log.Println("Received proxy packet for the subnetwork ", proxy.ProxyIpSubnetwork.IP.String())
				log.Println("From src ip ", ipv4.SrcIP, " to dst ip ", ipv4.DstIP)

				//Check proxy cache
				entry, exist := proxy.cache.RetrieveByServiceIP(ipv4.SrcIP, int(tcp.SrcPort), ipv4.DstIP)
				if !exist {
					//If no cache entry ask to the environment for a TableQuery
					tableEntryList := proxy.environment.GetTableEntryByServiceIP(ipv4.DstIP)

					//If no table entry available
					if len(tableEntryList) < 1 {
						//discard packet
						return packet
					}

					//Choose between the table entry according to the ServiceIP algorithm
					tableEntry := tableEntryList[rand.Intn(len(tableEntryList))]
					//TODO smart ServiceIP algorithms

					//Update cache
					entry = ConversionEntry{
						srcip:        ipv4.SrcIP,
						dstip:        tableEntry.Nsip,
						dstServiceIp: ipv4.DstIP,
						srcport:      int(tcp.SrcPort),
						dstport:      int(tcp.DstPort),
					}
					proxy.cache.Add(entry)
				}
				return OutgoingConversion(entry.dstip, entry.srcport, packet)

			}
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

			//Check proxy cache for REVERSE entry conversion
			//Dstip->srcip, DstPort->srcport, SrcIP -> dstIP
			entry, exist := proxy.cache.RetrieveByIp(ipv4.DstIP, int(tcp.DstPort), ipv4.SrcIP)
			if !exist {
				//No proxy cache entry, no translation needed
				return packet
			}

			log.Println("Received a proxy packet to port ", proxy.TunnelPort)
			log.Println("From src ip ", ipv4.SrcIP, " to dst ip ", ipv4.DstIP)
			//Reverse conversion
			return IngoingConversion(entry.dstServiceIp, entry.srcport, packet)

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

	log.Println("Bringing tun up with addr " + proxy.tunNetIP)
	cmd := exec.Command("ip", "addr", "add", proxy.tunNetIP, "dev", ifce.Name())
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
	log.Println("Increasing the MTU on the TUN dev")
	cmd = exec.Command("ip", "link", "set", "dev", ifce.Name(), "mtu", "1492")
	_, err = cmd.Output()
	if err != nil {
		log.Fatal(err.Error())
	}

	//Add network routing rule, Done by default by the system
	log.Println("adding routing rule for 172.19.0.0/16 to " + ifce.Name())
	cmd = exec.Command("ip", "route", "add", "172.19.0.0/16", "dev", ifce.Name())
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

	proxy.HostTUNDeviceName = ifce.Name()
	proxy.ifce = ifce
	proxy.listenConnection = lstnConn
}

// Enable listening to outgoing packets
// if the goroutine must be stopped, send true to the stop channel
// when the channels finish listening a "true" is sent back to the finish channel
// in case of fatal error they are routed back to the err channel
func (proxy *GoProxyTunnel) tunOutgoingListen() {
	readoutput := make(chan []byte)
	readerror := make(chan error)
	go ifaceread(proxy.ifce, readoutput, readerror)
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
			go ifaceread(proxy.ifce, readoutput, readerror)
		case msg := <-readoutput:
			//restart the interface read
			go ifaceread(proxy.ifce, readoutput, readerror)
			//invoke the handler function for outgoing packets
			packet := gopacket.NewPacket(msg, layers.LayerTypeIPv4, gopacket.Default)
			go proxy.outgoingMessage(packet)
		}
	}
}

// Enable listening for ingoing packets
// if the goroutine must be stopped, send true to the stop channel
// when the channels finish listening a "true" is sent back to the finish channel
// in case of fatal error they are routed back to the err channel
func (proxy *GoProxyTunnel) tunIngoingListen() {
	readoutput := make(chan []byte)
	readerror := make(chan error)
	go udpread(proxy.listenConnection, readoutput, readerror)
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
			go udpread(proxy.listenConnection, readoutput, readerror)
		case msg := <-readoutput:
			//restart the interface read
			go udpread(proxy.listenConnection, readoutput, readerror)
			//invoke the handler function for ingoing packets
			packet := gopacket.NewPacket(msg, layers.LayerTypeIPv4, gopacket.Default)
			go proxy.ingoingMessage(packet)
		}
	}
}

//Given a network namespace IP find the machine IP and port for the tunneling
func (proxy *GoProxyTunnel) locateRemoteAddress(nsIP net.IP) (net.IP, int) {

	log.Println("Locating Remote host address for NsIp: ", nsIP.String())

	//convert namespace IP to host IP
	tableElement, found := proxy.environment.GetTableEntryByNsIP(nsIP)
	if found {
		log.Println("Remote NS IP", nsIP.String(), " translated to ", tableElement.Nodeip.String())
		return tableElement.Nodeip, tableElement.Nodeport
	}

	//If nothing found, just let the packet to be dropped
	return nsIP, -1

}

//forward message to final destination
func (proxy *GoProxyTunnel) forward(dstHost net.IP, dstPort int, packet gopacket.Packet) {

	//ipLayer := packet.Layer(layers.LayerTypeIPv4);
	l4Layer := packet.Layer(layers.LayerTypeTCP)
	istcp := true
	if l4Layer == nil {
		l4Layer = packet.Layer(layers.LayerTypeUDP)
		if l4Layer == nil {
			return
		}
		istcp = false
	}

	//If destination address is local machine no need for tunneling
	if dstHost.Equal(proxy.localIP) {
		log.Println("Forwarding packet locally")
		//convert srcip
		if istcp {
			//
		} else {

		}
	}

	//If destination address is a rmeote machine -> tunneling
	remoteAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%v", dstHost, dstPort))
	if nil != err {
		log.Println("[ERROR] Unable to resolve remote addr:", err)
		//TODO: add fallback mechanism
		return
	}
	_, err = proxy.listenConnection.WriteToUDP(packetToByte(packet), remoteAddr)
	if err != nil {
		log.Println("[ERROR]", err)
	}
}

// read output from an interface and wrap the read operation with a channel
// out channel gives back the byte array of the output
// errchannel is the channel where in case of error the error is routed
func ifaceread(ifce *water.Interface, out chan<- []byte, errchannel chan<- error) {
	packet := make([]byte, 2000)
	n, err := ifce.Read(packet)
	if err != nil {
		errchannel <- err
	}
	out <- packet[:n]
}

// read output from an UDP connection and wrap the read operation with a channel
// out channel gives back the byte array of the output
// errchannel is the channel where in case of error the error is routed
func udpread(conn *net.UDPConn, out chan<- []byte, errchannel chan<- error) {
	packet := make([]byte, 2000)
	n, _, err := conn.ReadFromUDP(packet)
	if err != nil {
		errchannel <- err
	}
	out <- packet[:n]
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

func OutgoingConversion(dstIp net.IP, sourcePort int, packet gopacket.Packet) gopacket.Packet {

	ip := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	ip.DstIP = dstIp

	tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
	tcp.SrcPort = layers.TCPPort(sourcePort)

	return serializeTcpPacket(tcp, ip, packet)
}

func IngoingConversion(srcIP net.IP, dstPort int, packet gopacket.Packet) gopacket.Packet {

	ip := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	ip.SrcIP = srcIP

	tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
	tcp.DstPort = layers.TCPPort(dstPort)

	return serializeTcpPacket(tcp, ip, packet)
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

// GetLocalIP returns the non loopback local IP of the host
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}
