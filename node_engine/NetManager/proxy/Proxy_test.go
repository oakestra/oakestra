package proxy

import (
	"../env"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"net"
	"testing"
)

type FakeEnv struct {
}

func (fakeenv *FakeEnv) GetTableEntryByServiceIP(ip net.IP) []env.TableEntry {
	entrytable := make([]env.TableEntry, 0)
	//If entry already available
	entry := env.TableEntry{
		Appname:          "a",
		Appns:            "a",
		Servicename:      "b",
		Servicenamespace: "b",
		Instancenumber:   0,
		Cluster:          0,
		Nodeip:           net.ParseIP("10.0.0.1"),
		Nsip:             net.ParseIP("172.19.2.12"),
		ServiceIP: []env.ServiceIP{{
			IpType:  env.Closest,
			Address: net.ParseIP("172.30.255.255"),
		}},
	}
	entrytable = append(entrytable, entry)
	return entrytable
}

func (fakeenv *FakeEnv) GetTableEntryByNsIP(ip net.IP) (env.TableEntry, bool) {
	return env.TableEntry{}, false
}

func getFakeTunnel() GoProxyTunnel {
	tunnel := GoProxyTunnel{
		tunNetIP:    "172.19.1.254/16",
		ifce:        nil,
		isListening: true,
		ProxyIpSubnetwork: net.IPNet{
			IP:   net.ParseIP("172.30.0.0"),
			Mask: net.IPMask(net.ParseIP("255.255.0.0").To4()),
		},
		HostTUNDeviceName: "goProxyTun",
		TunnelPort:        50011,
		listenConnection:  nil,
		cache:             NewProxyCache(),
	}
	tunnel.SetEnvironment(&FakeEnv{})
	return tunnel
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

func TestOutgoingProxy(t *testing.T) {
	proxy := getFakeTunnel()

	proxypacket := getFakePacket("172.19.1.1", "172.30.255.255", 666, 80)
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
			//tcp, _ := tcpLayer.(*layers.TCP)
			//if !(tcp.SrcPort == layers.TCPPort(proxy.TunnelPort)) {
			//	t.Error("srcPort = ", tcp.SrcPort.String(), "; want = ", proxy.TunnelPort)
			//}
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

	proxypacket := getFakePacket("172.19.2.1", "172.19.1.15", 666, 777)
	noproxypacket := getFakePacket("172.19.2.1", "172.19.1.12", 666, 80)

	//update proxy cache
	entry := ConversionEntry{
		srcip:        net.ParseIP("172.19.1.15"),
		dstip:        net.ParseIP("172.19.2.1"),
		dstServiceIp: net.ParseIP("172.30.255.255"),
		srcport:      777,
		dstport:      666,
	}
	proxy.cache.Add(entry)

	newpacketproxy := proxy.ingoingProxy(proxypacket)
	newpacketnoproxy := proxy.ingoingProxy(noproxypacket)

	if ipLayer := newpacketproxy.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		if tcpLayer := newpacketproxy.Layer(layers.LayerTypeTCP); tcpLayer != nil {

			ipv4, _ := ipLayer.(*layers.IPv4)
			srcexpected := net.ParseIP("172.30.255.255")
			if !ipv4.SrcIP.Equal(srcexpected) {
				t.Error("srcIp = ", ipv4.SrcIP.String(), "; want =", srcexpected)
			}

			//tcp, _ := tcpLayer.(*layers.TCP)
			//if !(int(tcp.DstPort) == entry.srcport) {
			//	t.Error("dstPort = ", int(tcp.DstPort), "; want = ", entry.srcport)
			//}
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
