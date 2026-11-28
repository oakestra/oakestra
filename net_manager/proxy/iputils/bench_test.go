package iputils

import (
	"net/netip"
	"testing"

	"github.com/google/gopacket/layers"
)

// realisticPayload approximates a full-size TCP segment under the overlay's
// 1450-byte MTU.
func realisticPayload() []byte {
	p := make([]byte, 1400)
	for i := range p {
		p[i] = byte(i)
	}
	return p
}

func BenchmarkParseV4(b *testing.B) {
	wire := buildIPv4(b, "10.19.1.1", "10.30.255.255",
		&layers.TCP{SrcPort: 40000, DstPort: 80, SYN: true}, realisticPayload())

	b.ReportAllocs()
	for b.Loop() {
		if _, ok := Parse(wire); !ok {
			b.Fatal("parse failed")
		}
	}
}

func BenchmarkRewriteV4(b *testing.B) {
	wire := buildIPv4(b, "10.19.1.1", "10.30.255.255",
		&layers.TCP{SrcPort: 40000, DstPort: 80, SYN: true}, realisticPayload())
	newSrc := netip.MustParseAddr("10.30.255.254")
	newDst := netip.MustParseAddr("10.19.2.12")

	buf := make([]byte, len(wire))
	b.ReportAllocs()
	for b.Loop() {
		copy(buf, wire)
		pkt, ok := Parse(buf)
		if !ok {
			b.Fatal("parse failed")
		}
		if !pkt.Rewrite(newSrc, newDst) {
			b.Fatal("rewrite failed")
		}
	}
}

func BenchmarkRewriteV6(b *testing.B) {
	wire := buildIPv6(b, "fc00::1", "fdff:2000::ff",
		&layers.TCP{SrcPort: 40000, DstPort: 80, SYN: true}, realisticPayload())
	newSrc := netip.MustParseAddr("fdff::fe")
	newDst := netip.MustParseAddr("fd00::12")

	buf := make([]byte, len(wire))
	b.ReportAllocs()
	for b.Loop() {
		copy(buf, wire)
		pkt, ok := Parse(buf)
		if !ok {
			b.Fatal("parse failed")
		}
		if !pkt.Rewrite(newSrc, newDst) {
			b.Fatal("rewrite failed")
		}
	}
}
