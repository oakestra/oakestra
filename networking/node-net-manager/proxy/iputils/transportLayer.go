package iputils

import (
	"github.com/google/gopacket/layers"
)

type TransportLayer struct {
	TransportLayerProtocol
}

type TransportLayerProtocol interface {
	GetSourcePort() uint16
	GetDestPort() uint16
	GetProtocol() string
	GetUDPLayer() *layers.UDP
	GetTCPLayer() *layers.TCP
}

type UDPLayer struct {
	*layers.UDP
}

type TCPLayer struct {
	*layers.TCP
}

func (l UDPLayer) GetSourcePort() uint16 {
	return uint16(l.SrcPort)
}

func (l UDPLayer) GetDestPort() uint16 {
	return uint16(l.DstPort)
}

func (l TCPLayer) GetSourcePort() uint16 {
	return uint16(l.SrcPort)
}

func (l TCPLayer) GetDestPort() uint16 {
	return uint16(l.DstPort)
}

func (l TCPLayer) GetProtocol() string {
	return "TCP"
}

func (l UDPLayer) GetProtocol() string {
	return "UDP"
}

func (l UDPLayer) GetUDPLayer() *layers.UDP {
	return l.UDP
}

func (l UDPLayer) GetTCPLayer() *layers.TCP {
	return nil
}

func (l TCPLayer) GetUDPLayer() *layers.UDP {
	return nil
}

func (l TCPLayer) GetTCPLayer() *layers.TCP {
	return l.TCP
}
