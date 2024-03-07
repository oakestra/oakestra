package iputils

import (
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

type NetworkLayerPacket interface {
	isNetworkLayer() bool
	GetLayer() gopacket.Layer
	DecodeNetworkLayer(p gopacket.Packet)
	GetTransportLayer() TransportLayerProtocol
	Defragment() error
	GetSrcIP() net.IP
	GetDestIP() net.IP
	GetProtocolVersion() uint8
	GetNextHeader() uint8
	SerializePacket(net.IP, net.IP, TransportLayerProtocol) gopacket.Packet
	serializeUDPHeader(*layers.UDP) gopacket.Packet
	serializeTCPHeader(*layers.TCP) gopacket.Packet
}

type NetworkLayer struct {
	NetworkLayerPacket
}

func NewNetworkLayerPacket(ipt layers.IPProtocol, nl gopacket.NetworkLayer) NetworkLayerPacket {
	if ipt == layers.IPProtocolIPv4 {
		return newIPv4Packet(nl)
	}
	if ipt == layers.IPProtocolIPv6 {
		return newIPv6Packet(nl)
	}
	return nil
}

func NewGoPacket(bytes []byte, ipt layers.IPProtocol) gopacket.Packet {
	if ipt == layers.IPProtocolIPv4 {
		return gopacket.NewPacket(bytes, layers.LayerTypeIPv4, gopacket.Default)
	}
	if ipt == layers.IPProtocolIPv6 {
		return gopacket.NewPacket(bytes, layers.LayerTypeIPv6, gopacket.Default)
	}
	return nil
}
