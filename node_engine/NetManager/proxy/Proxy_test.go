package proxy

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"net"
	"testing"
)

func getFakeTunnel() GoProxyTunnel {
	return GoProxyTunnel{
		tunNetIP:    "172.19.1.254/16",
		ifce:        nil,
		isListening: true,
		ProxyIpSubnetwork: net.IPNet{
			IP:   net.ParseIP("172.32.0.0"),
			Mask: net.IPMask(net.ParseIP("255.255.0.0").To4()),
		},
		HostTUNDeviceName: "goProxyTun",
		TunnelPort:        50011,
		listenConnection:  nil,
	}
}

func getFakePacket(srcIP string, dstIP string, srcPort int, dstPort int) gopacket.Packet {
	ipLayer := layers.IPv4{
		SrcIP:    net.ParseIP(srcIP),
		DstIP:    net.ParseIP(dstIP),
		Protocol: layers.IPProtocolTCP,
	}
	tcpLayer := layers.TCP{
		SrcPort: layers.TCPPort(srcPort),
		DstPort: layers.TCPPort(dstPort),
		SYN:     true,
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: false,
	}
	_ = gopacket.SerializeLayers(buf, opts, &ipLayer, &tcpLayer)
	return gopacket.NewPacket(buf.Bytes(), layers.LayerTypeIPv4, gopacket.Default)
}

func TestOutgoingProxyConversion(t *testing.T) {
	packet := getFakePacket("172.19.1.1", "172.32.1.1", 666, 80)

	dstIp := net.ParseIP("172.19.8.12")
	sourcePort := 50011

	newpacket := OutgoingConversion(dstIp, sourcePort, packet)

	if ipLayer := newpacket.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		if tcpLayer := newpacket.Layer(layers.LayerTypeTCP); tcpLayer != nil {

			ipv4, _ := ipLayer.(*layers.IPv4)
			if !ipv4.DstIP.Equal(dstIp) {
				t.Error("dstIP = ", ipv4.DstIP.String(), "; want =", dstIp)
			}

			tcp, _ := tcpLayer.(*layers.TCP)
			if !(tcp.SrcPort == layers.TCPPort(sourcePort)) {
				t.Error("srcPort = ", tcp.SrcPort.String(), "; want = ", sourcePort)
			}
		}
	}
}

func TestIngoingProxyConversion(t *testing.T) {
	packet := getFakePacket("172.19.8.12", "172.19.1.1", 4376, 50011)

	srcIp := net.ParseIP("172.32.1.1")
	dstPort := 666

	newpacket := IngoingConversion(srcIp, dstPort, packet)

	if ipLayer := newpacket.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		if tcpLayer := newpacket.Layer(layers.LayerTypeTCP); tcpLayer != nil {

			ipv4, _ := ipLayer.(*layers.IPv4)
			if !ipv4.SrcIP.Equal(srcIp) {
				t.Error("srcIp = ", ipv4.SrcIP.String(), "; want =", srcIp)
			}

			tcp, _ := tcpLayer.(*layers.TCP)
			if !(tcp.DstPort == layers.TCPPort(dstPort)) {
				t.Error("dstPort = ", tcp.DstPort.String(), "; want = ", dstPort)
			}
		}
	}
}

func TestOutgoingProxy(t *testing.T) {
	proxy := getFakeTunnel()

	proxypacket := getFakePacket("172.19.1.1", "172.32.255.255", 666, 80)
	noproxypacket := getFakePacket("172.19.1.1", "172.20.1.1", 666, 80)

	newpacketproxy := proxy.outgoingProxy(proxypacket)
	newpacketnoproxy := proxy.outgoingProxy(noproxypacket)

	if ipLayer := newpacketproxy.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		if tcpLayer := newpacketproxy.Layer(layers.LayerTypeTCP); tcpLayer != nil {

			ipv4, _ := ipLayer.(*layers.IPv4)
			dstexpected := net.ParseIP("172.19.2.12")
			if !ipv4.DstIP.Equal(dstexpected) {
				t.Error("dstIP = ", ipv4.DstIP.String(), "; want =", dstexpected)
			}

			tcp, _ := tcpLayer.(*layers.TCP)
			if !(tcp.SrcPort == layers.TCPPort(proxy.TunnelPort)) {
				t.Error("srcPort = ", tcp.SrcPort.String(), "; want = ", proxy.TunnelPort)
			}
		}
	}
	if ipLayer := newpacketnoproxy.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		if tcpLayer := newpacketnoproxy.Layer(layers.LayerTypeTCP); tcpLayer != nil {

			ipv4, _ := ipLayer.(*layers.IPv4)
			dstexpected := net.ParseIP("172.20.1.1")
			if !ipv4.DstIP.Equal(dstexpected) {
				t.Error("dstIP = ", ipv4.DstIP.String(), "; want =", dstexpected)
			}

			tcp, _ := tcpLayer.(*layers.TCP)
			if !(tcp.SrcPort == layers.TCPPort(666)) {
				t.Error("srcPort = ", tcp.SrcPort.String(), "; want = ", 666)
			}
		}
	}
}

func TestIngoingProxy(t *testing.T) {
	proxy := getFakeTunnel()
	proxy.bufferPort = 666

	proxypacket := getFakePacket("172.19.2.1", "172.19.1.15", 666, proxy.TunnelPort)
	noproxypacket := getFakePacket("172.19.2.1", "172.19.1.12", 666, 80)

	newpacketproxy := proxy.ingoingProxy(proxypacket)
	newpacketnoproxy := proxy.ingoingProxy(noproxypacket)

	if ipLayer := newpacketproxy.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		if tcpLayer := newpacketproxy.Layer(layers.LayerTypeTCP); tcpLayer != nil {

			ipv4, _ := ipLayer.(*layers.IPv4)
			srcexpected := net.ParseIP("172.32.255.255")
			if !ipv4.SrcIP.Equal(srcexpected) {
				t.Error("srcIp = ", ipv4.SrcIP.String(), "; want =", srcexpected)
			}

			tcp, _ := tcpLayer.(*layers.TCP)
			if !(tcp.DstPort == layers.TCPPort(proxy.bufferPort)) {
				t.Error("dstPort = ", tcp.DstPort.String(), "; want = ", proxy.bufferPort)
			}
		}
	}
	if ipLayer := newpacketnoproxy.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		if tcpLayer := newpacketnoproxy.Layer(layers.LayerTypeTCP); tcpLayer != nil {

			ipv4, _ := ipLayer.(*layers.IPv4)
			srcexpected := net.ParseIP("172.19.2.1")
			if !ipv4.SrcIP.Equal(srcexpected) {
				t.Error("dstIP = ", ipv4.SrcIP.String(), "; want =", srcexpected)
			}

			tcp, _ := tcpLayer.(*layers.TCP)
			if !(tcp.DstPort == layers.TCPPort(80)) {
				t.Error("srcPort = ", tcp.DstPort.String(), "; want = ", 80)
			}
		}
	}
}
