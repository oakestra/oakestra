package proxy

import (
	"NetManager/TableEntryCache"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"math/rand"
	"net"
	"testing"
)

type FakeEnv struct {
}

func (fakeenv *FakeEnv) GetTableEntryByServiceIP(ip net.IP) []TableEntryCache.TableEntry {
	entrytable := make([]TableEntryCache.TableEntry, 0)
	//If entry already available
	entry := TableEntryCache.TableEntry{
		Appname:          "a",
		Appns:            "a",
		Servicename:      "b",
		Servicenamespace: "b",
		Instancenumber:   0,
		Cluster:          0,
		Nodeip:           net.ParseIP("10.0.0.1"),
		Nsip:             net.ParseIP("10.19.2.12"),
		ServiceIP: []TableEntryCache.ServiceIP{{
			IpType:  TableEntryCache.Closest,
			Address: net.ParseIP("10.30.255.255"),
		},
			{
				IpType:  TableEntryCache.InstanceNumber,
				Address: net.ParseIP("10.30.255.254"),
			}},
	}
	entrytable = append(entrytable, entry)
	return entrytable
}

func (fakeenv *FakeEnv) GetTableEntryByNsIP(ip net.IP) (TableEntryCache.TableEntry, bool) {
	entry := TableEntryCache.TableEntry{
		Appname:          "a",
		Appns:            "a",
		Servicename:      "c",
		Servicenamespace: "b",
		Instancenumber:   0,
		Cluster:          0,
		Nodeip:           net.ParseIP("10.0.0.1"),
		Nsip:             net.ParseIP("10.19.1.1"),
		ServiceIP: []TableEntryCache.ServiceIP{{
			IpType:  TableEntryCache.Closest,
			Address: net.ParseIP("10.30.255.252"),
		},
			{
				IpType:  TableEntryCache.InstanceNumber,
				Address: net.ParseIP("10.30.255.253"),
			}},
	}
	return entry, true
}

func (fakeenv *FakeEnv) GetTableEntryByInstanceIP(ip net.IP) (TableEntryCache.TableEntry, bool) {
	return TableEntryCache.TableEntry{}, false
}

func getFakeTunnel() GoProxyTunnel {
	tunnel := GoProxyTunnel{
		tunNetIP:    "10.19.1.254",
		ifce:        nil,
		isListening: true,
		ProxyIpSubnetwork: net.IPNet{
			IP:   net.ParseIP("10.30.0.0"),
			Mask: net.IPMask(net.ParseIP("255.255.0.0").To4()),
		},
		HostTUNDeviceName: "goProxyTun",
		TunnelPort:        50011,
		listenConnection:  nil,
		proxycache:        NewProxyCache(),
		randseed:          rand.New(rand.NewSource(42)),
	}
	tunnel.SetEnvironment(&FakeEnv{})
	return tunnel
}

func getFakePacket(srcIP string, dstIP string, srcPort int, dstPort int) (gopacket.Packet, *layers.IPv4, *layers.TCP) {
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
	return gopacket.NewPacket(buf.Bytes(), layers.LayerTypeIPv4, gopacket.Default), &ipLayer, &tcpLayer
}

func TestOutgoingProxy(t *testing.T) {
	proxy := getFakeTunnel()

	_, ip, tcp := getFakePacket("10.19.1.1", "10.30.255.255", 666, 80)
	_, noip, notcp := getFakePacket("10.19.1.1", "10.20.1.1", 666, 80)

	newpacketproxy := proxy.outgoingProxy(ip, tcp, nil)
	newpacketnoproxy := proxy.outgoingProxy(noip, notcp, nil)

	if ipLayer := newpacketproxy.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		if tcpLayer := newpacketproxy.Layer(layers.LayerTypeTCP); tcpLayer != nil {

			ipv4, _ := ipLayer.(*layers.IPv4)
			dstexpected := net.ParseIP("10.19.2.12")
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
			dstexpected := net.ParseIP("10.20.1.1")
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

	_, ip, tcp := getFakePacket("10.30.0.5", "10.19.1.15", 666, 777)
	_, noip, notcp := getFakePacket("10.19.2.1", "10.19.1.12", 666, 80)

	//update proxy proxycache
	entry := ConversionEntry{
		srcip:         net.ParseIP("10.19.1.15"),
		dstip:         net.ParseIP("10.19.2.1"),
		dstServiceIp:  net.ParseIP("10.30.255.255"),
		srcInstanceIp: net.ParseIP("10.30.0.50"),
		srcport:       777,
		dstport:       666,
	}
	proxy.proxycache.Add(entry)

	newpacketproxy := proxy.ingoingProxy(ip, tcp, nil)
	newpacketnoproxy := proxy.ingoingProxy(noip, notcp, nil)

	if ipLayer := newpacketproxy.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		if tcpLayer := newpacketproxy.Layer(layers.LayerTypeTCP); tcpLayer != nil {

			ipv4, _ := ipLayer.(*layers.IPv4)
			srcexpected := net.ParseIP("10.30.255.255")
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
			srcexpected := net.ParseIP("10.19.2.1")
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
