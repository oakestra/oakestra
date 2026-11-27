package iputils

import (
	"bytes"
	"net"
	"net/netip"
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// --- test fixtures, built with gopacket for convenience (test-only; the
// production code in this package never imports gopacket) ---

func buildIPv4(t testing.TB, srcIP, dstIP string, transport gopacket.SerializableLayer, payload []byte) []byte {
	t.Helper()
	ip := &layers.IPv4{
		Version: 4,
		IHL:     5,
		TTL:     64,
		SrcIP:   net.ParseIP(srcIP).To4(),
		DstIP:   net.ParseIP(dstIP).To4(),
	}
	switch tl := transport.(type) {
	case *layers.TCP:
		ip.Protocol = layers.IPProtocolTCP
		_ = tl.SetNetworkLayerForChecksum(ip)
	case *layers.UDP:
		ip.Protocol = layers.IPProtocolUDP
		_ = tl.SetNetworkLayerForChecksum(ip)
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, ip, transport, gopacket.Payload(payload)); err != nil {
		t.Fatalf("build IPv4 packet: %v", err)
	}
	return append([]byte(nil), buf.Bytes()...)
}

func buildIPv6(t testing.TB, srcIP, dstIP string, transport gopacket.SerializableLayer, payload []byte) []byte {
	t.Helper()
	ip := &layers.IPv6{
		Version:  6,
		HopLimit: 64,
		SrcIP:    net.ParseIP(srcIP),
		DstIP:    net.ParseIP(dstIP),
	}
	switch tl := transport.(type) {
	case *layers.TCP:
		ip.NextHeader = layers.IPProtocolTCP
		_ = tl.SetNetworkLayerForChecksum(ip)
	case *layers.UDP:
		ip.NextHeader = layers.IPProtocolUDP
		_ = tl.SetNetworkLayerForChecksum(ip)
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, ip, transport, gopacket.Payload(payload)); err != nil {
		t.Fatalf("build IPv6 packet: %v", err)
	}
	return append([]byte(nil), buf.Bytes()...)
}

// referenceTranslateV4/V6 independently reproduces what Rewrite should
// produce by going through gopacket's own (full-recompute) serializer, so
// Rewrite's incremental math can be checked byte-for-byte against a
// completely different implementation.
func referenceTranslateV4(t *testing.T, wire []byte, newSrc, newDst net.IP) []byte {
	t.Helper()
	pkt := gopacket.NewPacket(wire, layers.LayerTypeIPv4, gopacket.Default)
	ipLayer := pkt.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	ipLayer.SrcIP = newSrc
	ipLayer.DstIP = newDst

	var transport gopacket.SerializableLayer
	var payload []byte
	if tcpLayer := pkt.Layer(layers.LayerTypeTCP); tcpLayer != nil {
		tcp := tcpLayer.(*layers.TCP)
		_ = tcp.SetNetworkLayerForChecksum(ipLayer)
		transport, payload = tcp, tcp.Payload
	} else if udpLayer := pkt.Layer(layers.LayerTypeUDP); udpLayer != nil {
		udp := udpLayer.(*layers.UDP)
		_ = udp.SetNetworkLayerForChecksum(ipLayer)
		transport, payload = udp, udp.Payload
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: false, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, ipLayer, transport, gopacket.Payload(payload)); err != nil {
		t.Fatalf("reference reserialize: %v", err)
	}
	return append([]byte(nil), buf.Bytes()...)
}

func referenceTranslateV6(t *testing.T, wire []byte, newSrc, newDst net.IP) []byte {
	t.Helper()
	pkt := gopacket.NewPacket(wire, layers.LayerTypeIPv6, gopacket.Default)
	ipLayer := pkt.Layer(layers.LayerTypeIPv6).(*layers.IPv6)
	ipLayer.SrcIP = newSrc
	ipLayer.DstIP = newDst

	var transport gopacket.SerializableLayer
	var payload []byte
	if tcpLayer := pkt.Layer(layers.LayerTypeTCP); tcpLayer != nil {
		tcp := tcpLayer.(*layers.TCP)
		_ = tcp.SetNetworkLayerForChecksum(ipLayer)
		transport, payload = tcp, tcp.Payload
	} else if udpLayer := pkt.Layer(layers.LayerTypeUDP); udpLayer != nil {
		udp := udpLayer.(*layers.UDP)
		_ = udp.SetNetworkLayerForChecksum(ipLayer)
		transport, payload = udp, udp.Payload
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: false, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, ipLayer, transport, gopacket.Payload(payload)); err != nil {
		t.Fatalf("reference reserialize: %v", err)
	}
	return append([]byte(nil), buf.Bytes()...)
}

func mustAddr(t *testing.T, s string) netip.Addr {
	t.Helper()
	addr, err := netip.ParseAddr(s)
	if err != nil {
		t.Fatalf("parse addr %q: %v", s, err)
	}
	return addr
}

// --- cross-validation against an independent (full-recompute) implementation ---

func TestRewriteV4TCPMatchesFullRecompute(t *testing.T) {
	wire := buildIPv4(t, "10.19.1.5", "10.30.255.255",
		&layers.TCP{SrcPort: 40000, DstPort: 80, Seq: 12345, SYN: true, Window: 1024},
		[]byte("hello world, this is a test payload that is definitely not empty"))

	want := referenceTranslateV4(t, append([]byte(nil), wire...), net.ParseIP("10.30.255.254"), net.ParseIP("10.19.2.12"))

	got := append([]byte(nil), wire...)
	pkt, ok := Parse(got)
	if !ok || !pkt.HasTransport() {
		t.Fatalf("Parse failed or no transport (ok=%v)", ok)
	}
	if !pkt.Rewrite(mustAddr(t, "10.30.255.254"), mustAddr(t, "10.19.2.12")) {
		t.Fatal("Rewrite returned false")
	}
	if !bytes.Equal(pkt.Bytes(), want) {
		t.Errorf("mismatch:\n got  = % x\n want = % x", pkt.Bytes(), want)
	}
}

func TestRewriteV4UDPMatchesFullRecompute(t *testing.T) {
	wire := buildIPv4(t, "10.19.1.5", "10.30.255.255",
		&layers.UDP{SrcPort: 40000, DstPort: 53}, []byte("dns query payload here"))

	want := referenceTranslateV4(t, append([]byte(nil), wire...), net.ParseIP("10.30.255.254"), net.ParseIP("10.19.2.12"))

	got := append([]byte(nil), wire...)
	pkt, ok := Parse(got)
	if !ok || !pkt.HasTransport() {
		t.Fatalf("Parse failed or no transport (ok=%v)", ok)
	}
	if !pkt.Rewrite(mustAddr(t, "10.30.255.254"), mustAddr(t, "10.19.2.12")) {
		t.Fatal("Rewrite returned false")
	}
	if !bytes.Equal(pkt.Bytes(), want) {
		t.Errorf("mismatch:\n got  = % x\n want = % x", pkt.Bytes(), want)
	}
}

func TestRewriteV6TCPMatchesFullRecompute(t *testing.T) {
	wire := buildIPv6(t, "fc00::1", "fdff:2000::ff",
		&layers.TCP{SrcPort: 40000, DstPort: 80, Seq: 12345, SYN: true, Window: 1024},
		[]byte("hello world over ipv6, a longer payload for good measure"))

	want := referenceTranslateV6(t, append([]byte(nil), wire...), net.ParseIP("fdff::fe"), net.ParseIP("fd00::12"))

	got := append([]byte(nil), wire...)
	pkt, ok := Parse(got)
	if !ok || !pkt.HasTransport() {
		t.Fatalf("Parse failed or no transport (ok=%v)", ok)
	}
	if !pkt.Rewrite(mustAddr(t, "fdff::fe"), mustAddr(t, "fd00::12")) {
		t.Fatal("Rewrite returned false")
	}
	if !bytes.Equal(pkt.Bytes(), want) {
		t.Errorf("mismatch:\n got  = % x\n want = % x", pkt.Bytes(), want)
	}
}

func TestRewriteV6UDPMatchesFullRecompute(t *testing.T) {
	wire := buildIPv6(t, "fc00::1", "fdff:2000::ff", &layers.UDP{SrcPort: 40000, DstPort: 53}, []byte("dns over ipv6"))

	want := referenceTranslateV6(t, append([]byte(nil), wire...), net.ParseIP("fdff::fe"), net.ParseIP("fd00::12"))

	got := append([]byte(nil), wire...)
	pkt, ok := Parse(got)
	if !ok || !pkt.HasTransport() {
		t.Fatalf("Parse failed or no transport (ok=%v)", ok)
	}
	if !pkt.Rewrite(mustAddr(t, "fdff::fe"), mustAddr(t, "fd00::12")) {
		t.Fatal("Rewrite returned false")
	}
	if !bytes.Equal(pkt.Bytes(), want) {
		t.Errorf("mismatch:\n got  = % x\n want = % x", pkt.Bytes(), want)
	}
}

// --- RFC 768 / RFC 8200 special cases gopacket's own serializer doesn't model ---

func TestRewriteV4UDPZeroChecksumStaysZero(t *testing.T) {
	wire := buildIPv4(t, "10.19.1.5", "10.30.255.255", &layers.UDP{SrcPort: 40000, DstPort: 53}, []byte("payload"))
	// UDP checksum is at IHL(20)+6 for a option-less IPv4 header.
	wire[20+6], wire[20+7] = 0, 0

	pkt, ok := Parse(wire)
	if !ok || !pkt.HasTransport() {
		t.Fatalf("Parse failed or no transport (ok=%v)", ok)
	}
	if !pkt.Rewrite(mustAddr(t, "10.30.255.254"), mustAddr(t, "10.19.2.12")) {
		t.Fatal("Rewrite returned false")
	}
	if got := readUint16(pkt.Bytes()[20+6 : 20+8]); got != 0 {
		t.Errorf("IPv4 UDP checksum 0 (no checksum) must stay 0, got %#04x", got)
	}
}

func TestRewriteNeverProducesZeroUDPChecksum(t *testing.T) {
	// 0x0000 is invalid here for both families (no-checksum on IPv4/UDP,
	// forbidden on IPv6/UDP), so any adjustment that folds to zero must come
	// out as 0xffff instead. Sweep enough src/dst pairs to actually hit that case.
	for _, v6 := range []bool{false, true} {
		var wire []byte
		if v6 {
			wire = buildIPv6(t, "fc00::1", "fdff:2000::ff", &layers.UDP{SrcPort: 1, DstPort: 2}, []byte("x"))
		} else {
			wire = buildIPv4(t, "10.0.0.1", "10.0.0.2", &layers.UDP{SrcPort: 1, DstPort: 2}, []byte("x"))
		}
		for a := 1; a < 250; a += 37 {
			for b := 1; b < 250; b += 41 {
				got := append([]byte(nil), wire...)
				pkt, ok := Parse(got)
				if !ok || !pkt.HasTransport() {
					t.Fatalf("Parse failed (v6=%v)", v6)
				}
				var newSrc, newDst netip.Addr
				if v6 {
					newSrc = mustAddr(t, "fdff::1")
					newDst = mustAddr(t, netFmtV6(a, b))
				} else {
					newSrc = mustAddr(t, "10.30.0.1")
					newDst = mustAddr(t, netFmtV4(a, b))
				}
				if !pkt.Rewrite(newSrc, newDst) {
					t.Fatalf("Rewrite failed (v6=%v)", v6)
				}
				var csumOff int
				if v6 {
					csumOff = ipv6HeaderLen + 6
				} else {
					csumOff = ipv4HeaderMinLen + 6
				}
				if got := readUint16(pkt.Bytes()[csumOff : csumOff+2]); got == 0 {
					t.Fatalf("UDP checksum came out as literal 0 (v6=%v, a=%d, b=%d)", v6, a, b)
				}
			}
		}
	}
}

func netFmtV4(a, b int) string { return net.IPv4(10, 30, byte(a), byte(b)).String() }
func netFmtV6(a, b int) string {
	ip := net.ParseIP("fdff:9999::1")
	ip[14], ip[15] = byte(a), byte(b)
	return ip.String()
}

// --- fragmentation ---

func TestNonFirstFragmentHasNoTransport(t *testing.T) {
	// Non-first fragment (offset > 0): the bytes after the header are
	// payload continuation, not a TCP/UDP header.
	buf := make([]byte, 40)
	buf[0] = 0x45 // version 4, IHL 5
	buf[9] = 6    // protocol TCP
	writeUint16(buf[6:8], 1)
	// bogus bytes where a TCP header would be, to prove we never touch them as such
	copy(buf[20:], []byte{0xff, 0xff, 0xff, 0xff})

	pkt, ok := Parse(buf)
	if !ok {
		t.Fatal("Parse failed")
	}
	if pkt.HasTransport() {
		t.Error("a non-first fragment must not report HasTransport")
	}
	// Rewrite must still succeed (fixing only the IP header checksum) and
	// must not touch bytes past the IP header.
	before := append([]byte(nil), buf[20:]...)
	if !pkt.Rewrite(mustAddr(t, "10.0.0.9"), mustAddr(t, "10.0.0.10")) {
		t.Fatal("Rewrite should succeed for a fragment (IP-header-only fixup)")
	}
	if !bytes.Equal(pkt.Bytes()[20:], before) {
		t.Error("Rewrite must not touch bytes beyond the IP header on a non-first fragment")
	}
}

func TestFirstFragmentHasTransport(t *testing.T) {
	wire := buildIPv4(t, "10.19.1.5", "10.30.255.255", &layers.TCP{SrcPort: 1, DstPort: 2, SYN: true}, []byte("payload"))
	// fragment offset 0, MF bit set: this is the first of several fragments
	// and does carry the L4 header.
	writeUint16(wire[6:8], 0x2000)

	pkt, ok := Parse(wire)
	if !ok {
		t.Fatal("Parse failed")
	}
	if !pkt.HasTransport() {
		t.Error("the first fragment (offset 0) should still report HasTransport")
	}
}

// --- IPv6 extension headers ---

func TestIPv6HopByHopExtensionHeaderIsSkipped(t *testing.T) {
	tcp := []byte{0, 1, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0x50, 0x02, 0, 0, 0, 0, 0, 0}
	hopByHop := []byte{byte(layers.IPProtocolTCP), 0, 0, 0, 0, 0, 0, 0} // next=TCP, hdrExtLen=0 -> 8 bytes total

	buf := make([]byte, ipv6HeaderLen+len(hopByHop)+len(tcp))
	buf[0] = 0x60
	buf[6] = 0 // next header = Hop-by-Hop
	buf[7] = 64
	copy(buf[8:24], net.ParseIP("fc00::1").To16())
	copy(buf[24:40], net.ParseIP("fc00::2").To16())
	copy(buf[40:], hopByHop)
	copy(buf[40+len(hopByHop):], tcp)

	pkt, ok := Parse(buf)
	if !ok {
		t.Fatal("Parse failed")
	}
	if pkt.Protocol() != ProtoTCP {
		t.Errorf("expected protocol TCP after skipping Hop-by-Hop, got %d", pkt.Protocol())
	}
	if !pkt.HasTransport() {
		t.Error("expected HasTransport true after skipping the extension header")
	}
	if pkt.SrcPort() != 1 || pkt.DstPort() != 2 {
		t.Errorf("got ports %d/%d, want 1/2", pkt.SrcPort(), pkt.DstPort())
	}
}

// --- basic accessor sanity ---

func TestParseRejectsShortBuffer(t *testing.T) {
	if _, ok := Parse(nil); ok {
		t.Error("Parse should reject an empty buffer")
	}
	if _, ok := Parse([]byte{0x45, 0, 0}); ok {
		t.Error("Parse should reject a too-short IPv4 buffer")
	}
}

func TestParseRejectsUnknownVersion(t *testing.T) {
	buf := make([]byte, 20)
	buf[0] = 0x55 // version 5
	if _, ok := Parse(buf); ok {
		t.Error("Parse should reject an unknown IP version")
	}
}

// --- fragment detection ---

// TestUnfragmentedPacketIsNotAFragment matters as much as detecting real
// fragments: the proxy keeps per-datagram translation state for anything that
// reports IsFragment, so an ordinary packet must never look like one.
func TestUnfragmentedPacketIsNotAFragment(t *testing.T) {
	v4 := buildIPv4(t, "10.19.1.5", "10.30.255.255", &layers.TCP{SrcPort: 1, DstPort: 2, SYN: true}, []byte("payload"))
	pkt, ok := Parse(v4)
	if !ok {
		t.Fatal("Parse failed")
	}
	if pkt.IsFragment() {
		t.Error("an ordinary IPv4 packet must not report IsFragment")
	}

	v6 := buildIPv6(t, "fc00::1", "fdff::2", &layers.TCP{SrcPort: 1, DstPort: 2, SYN: true}, []byte("payload"))
	pkt6, ok := Parse(v6)
	if !ok {
		t.Fatal("Parse failed")
	}
	if pkt6.IsFragment() {
		t.Error("an ordinary IPv6 packet must not report IsFragment")
	}
}

func TestIPv4FragmentAccessors(t *testing.T) {
	wire := buildIPv4(t, "10.19.1.5", "10.30.255.255", &layers.UDP{SrcPort: 1, DstPort: 2}, []byte("payload"))
	writeUint16(wire[4:6], 0xbeef) // identification

	t.Run("first", func(t *testing.T) {
		first := append([]byte(nil), wire...)
		writeUint16(first[6:8], 0x2000) // MF, offset 0
		pkt, ok := Parse(first)
		if !ok {
			t.Fatal("Parse failed")
		}
		if !pkt.IsFragment() || !pkt.IsFirstFragment() {
			t.Errorf("IsFragment=%v IsFirstFragment=%v; want true/true", pkt.IsFragment(), pkt.IsFirstFragment())
		}
		if pkt.FragmentID() != 0xbeef {
			t.Errorf("FragmentID = %#x; want 0xbeef", pkt.FragmentID())
		}
	})

	t.Run("last", func(t *testing.T) {
		last := append([]byte(nil), wire...)
		writeUint16(last[6:8], 128) // offset 128*8, MF clear: the final fragment
		pkt, ok := Parse(last)
		if !ok {
			t.Fatal("Parse failed")
		}
		if !pkt.IsFragment() || pkt.IsFirstFragment() {
			t.Errorf("IsFragment=%v IsFirstFragment=%v; want true/false", pkt.IsFragment(), pkt.IsFirstFragment())
		}
		if pkt.FragmentID() != 0xbeef {
			t.Errorf("FragmentID = %#x; want 0xbeef", pkt.FragmentID())
		}
	})
}

func TestIPv6FragmentAccessors(t *testing.T) {
	// IPv6 header -> Fragment header -> UDP
	udp := []byte{0, 1, 0, 2, 0, 8, 0, 0}
	build := func(offset uint16, more bool) []byte {
		buf := make([]byte, ipv6HeaderLen+8+len(udp))
		buf[0] = 0x60
		buf[6] = 44 // Fragment header
		buf[7] = 64
		writeUint16(buf[4:6], uint16(8+len(udp)))
		copy(buf[8:24], net.ParseIP("fc00::1").To16())
		copy(buf[24:40], net.ParseIP("fdff::2").To16())
		buf[40] = ProtoUDP
		offsetAndFlags := offset << 3
		if more {
			offsetAndFlags |= 1
		}
		writeUint16(buf[42:44], offsetAndFlags)
		buf[44], buf[45], buf[46], buf[47] = 0xde, 0xad, 0xbe, 0xef
		copy(buf[48:], udp)
		return buf
	}

	first, ok := Parse(build(0, true))
	if !ok {
		t.Fatal("Parse failed for the first IPv6 fragment")
	}
	if !first.IsFragment() || !first.IsFirstFragment() {
		t.Errorf("first fragment: IsFragment=%v IsFirstFragment=%v; want true/true",
			first.IsFragment(), first.IsFirstFragment())
	}
	if first.Protocol() != ProtoUDP {
		t.Errorf("protocol %d after the fragment header; want UDP", first.Protocol())
	}
	if !first.HasTransport() {
		t.Error("the first IPv6 fragment carries the transport header")
	}
	if first.FragmentID() != 0xdeadbeef {
		t.Errorf("FragmentID = %#x; want 0xdeadbeef", first.FragmentID())
	}

	later, ok := Parse(build(16, false))
	if !ok {
		t.Fatal("Parse failed for the later IPv6 fragment")
	}
	if !later.IsFragment() || later.IsFirstFragment() {
		t.Errorf("later fragment: IsFragment=%v IsFirstFragment=%v; want true/false",
			later.IsFragment(), later.IsFirstFragment())
	}
	if later.HasTransport() {
		t.Error("a later IPv6 fragment must not report HasTransport")
	}
	if later.FragmentID() != 0xdeadbeef {
		t.Errorf("FragmentID = %#x; want 0xdeadbeef", later.FragmentID())
	}
}
