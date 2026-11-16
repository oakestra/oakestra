package proxy

import (
	"NetManager/clock"
	"NetManager/proxy/iputils"
	"bytes"
	"encoding/binary"
	"testing"
)

// fragmentIPv4 splits a datagram the way the kernel would: the transport
// header only survives on the first fragment. splitAt is an offset into the
// payload and must be a multiple of 8.
func fragmentIPv4(t testing.TB, wire []byte, splitAt int) (first, later []byte) {
	t.Helper()
	if splitAt%8 != 0 {
		t.Fatalf("fragment offset %d is not a multiple of 8", splitAt)
	}
	header, payload := wire[:20], wire[20:]
	if splitAt >= len(payload) {
		t.Fatalf("split offset %d beyond payload length %d", splitAt, len(payload))
	}

	build := func(body []byte, flagsAndOffset uint16) []byte {
		frag := make([]byte, 20+len(body))
		copy(frag, header)
		copy(frag[20:], body)
		binary.BigEndian.PutUint16(frag[2:4], uint16(len(frag)))
		binary.BigEndian.PutUint16(frag[6:8], flagsAndOffset)
		binary.BigEndian.PutUint16(frag[10:12], 0)
		binary.BigEndian.PutUint16(frag[10:12], ipv4HeaderChecksum(frag[:20]))
		return frag
	}

	const moreFragments = 0x2000
	return build(payload[:splitAt], moreFragments), build(payload[splitAt:], uint16(splitAt/8))
}

// fragmentIPv6 does the same for IPv6, where fragmentation is expressed with
// an extension header rather than fields in the fixed header.
func fragmentIPv6(t testing.TB, wire []byte, splitAt int, id uint32) (first, later []byte) {
	t.Helper()
	if splitAt%8 != 0 {
		t.Fatalf("fragment offset %d is not a multiple of 8", splitAt)
	}
	header, payload := wire[:40], wire[40:]
	nextHeader := header[6]

	build := func(body []byte, offset uint16, more bool) []byte {
		frag := make([]byte, 40+8+len(body))
		copy(frag, header)
		frag[6] = 44 // Fragment header
		binary.BigEndian.PutUint16(frag[4:6], uint16(8+len(body)))
		frag[40] = nextHeader
		frag[41] = 0
		offsetAndFlags := offset << 3
		if more {
			offsetAndFlags |= 1
		}
		binary.BigEndian.PutUint16(frag[42:44], offsetAndFlags)
		binary.BigEndian.PutUint32(frag[44:48], id)
		copy(frag[48:], body)
		return frag
	}

	return build(payload[:splitAt], 0, true), build(payload[splitAt:], uint16(splitAt/8), false)
}

func ipv4HeaderChecksum(header []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(header); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(header[i : i+2]))
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

func largePayload(n int) []byte {
	p := make([]byte, n)
	for i := range p {
		p[i] = byte(i)
	}
	return p
}

// TestOutgoingFragmentedDatagram checks a later fragment replays the first
// fragment's translation and reassembles correctly at the far end.
func TestOutgoingFragmentedDatagram(t *testing.T) {
	dp := getFakeDatapath()
	wire := buildUDPv4(t, clientNsIP, serverVIP, 40000, 53, largePayload(2000))
	first, later := fragmentIPv4(t, wire, 1024)
	laterPayloadBefore := append([]byte(nil), later[20:]...)

	firstPkt := parseTestPacket(t, first)
	if !firstPkt.IsFragment() || !firstPkt.IsFirstFragment() {
		t.Fatal("first fragment misdetected")
	}
	node, port, _, ok := dp.outgoingProxy(&firstPkt)
	if !ok {
		t.Fatal("first fragment should have been proxied")
	}
	dp.proxycache.frags.remember(fragmentKey{
		src: mustAddr(clientNsIP), dst: mustAddr(serverVIP),
		id: firstPkt.FragmentID(), version: 4, proto: iputils.ProtoUDP,
	}, fragmentTranslation{
		newSrc: firstPkt.SrcIP(), newDst: firstPkt.DstIP(),
		dstNode: node, dstNodePort: port,
	})

	laterPkt, ok := iputils.Parse(later)
	if !ok {
		t.Fatal("later fragment failed to parse")
	}
	if !laterPkt.IsFragment() || laterPkt.IsFirstFragment() {
		t.Fatal("later fragment misdetected")
	}
	if laterPkt.HasTransport() {
		t.Fatal("a later fragment must never be treated as carrying a transport header")
	}
	if !isLaterFragment(&laterPkt) {
		t.Fatal("isLaterFragment should recognise this")
	}

	translation, known := dp.proxycache.frags.lookup(keyFor(&laterPkt))
	if !known {
		t.Fatal("no fragment state recorded for the later fragment")
	}
	if !laterPkt.Rewrite(translation.newSrc, translation.newDst) {
		t.Fatal("later fragment rewrite failed")
	}

	if laterPkt.SrcIP() != firstPkt.SrcIP() || laterPkt.DstIP() != firstPkt.DstIP() {
		t.Errorf("later fragment translated to %s -> %s; want %s -> %s",
			laterPkt.SrcIP(), laterPkt.DstIP(), firstPkt.SrcIP(), firstPkt.DstIP())
	}
	if translation.dstNode != node || translation.dstNodePort != port {
		t.Errorf("later fragment routed to %s:%d; want %s:%d",
			translation.dstNode, translation.dstNodePort, node, port)
	}
	if got := ipv4HeaderChecksum(laterPkt.Bytes()[:20]); got != 0 {
		t.Errorf("IPv4 header checksum does not verify (residual %#04x)", got)
	}
	// no transport header to patch here, so the payload bytes must be untouched.
	if !bytes.Equal(laterPkt.Bytes()[20:], laterPayloadBefore) {
		t.Error("later fragment payload was modified")
	}
}

func TestFragmentedDatagramIPv6(t *testing.T) {
	dp := getFakeDatapath()
	full := buildUDPv6(t, clientNsIPv6, serverVIPv6, 40000, 53, largePayload(2000))
	first, later := fragmentIPv6(t, full, 1024, 0xdeadbeef)
	laterPayloadBefore := append([]byte(nil), later[48:]...)

	firstPkt := parseTestPacket(t, first)
	if !firstPkt.IsFragment() || !firstPkt.IsFirstFragment() {
		t.Fatal("first IPv6 fragment misdetected")
	}
	if firstPkt.FragmentID() != 0xdeadbeef {
		t.Errorf("fragment id = %#x; want 0xdeadbeef", firstPkt.FragmentID())
	}
	key := keyFor(&firstPkt)
	node, port, _, ok := dp.outgoingProxy(&firstPkt)
	if !ok {
		t.Fatal("first IPv6 fragment should have been proxied")
	}
	dp.proxycache.frags.remember(key, fragmentTranslation{
		newSrc: firstPkt.SrcIP(), newDst: firstPkt.DstIP(),
		dstNode: node, dstNodePort: port,
	})

	laterPkt, ok := iputils.Parse(later)
	if !ok {
		t.Fatal("later IPv6 fragment failed to parse")
	}
	if !isLaterFragment(&laterPkt) {
		t.Fatal("later IPv6 fragment misdetected")
	}
	translation, known := dp.proxycache.frags.lookup(keyFor(&laterPkt))
	if !known {
		t.Fatal("no fragment state recorded for the later IPv6 fragment")
	}
	if !laterPkt.Rewrite(translation.newSrc, translation.newDst) {
		t.Fatal("later IPv6 fragment rewrite failed")
	}
	if laterPkt.SrcIP() != firstPkt.SrcIP() || laterPkt.DstIP() != firstPkt.DstIP() {
		t.Errorf("later fragment translated to %s -> %s; want %s -> %s",
			laterPkt.SrcIP(), laterPkt.DstIP(), firstPkt.SrcIP(), firstPkt.DstIP())
	}
	if !bytes.Equal(laterPkt.Bytes()[48:], laterPayloadBefore) {
		t.Error("later IPv6 fragment payload was modified")
	}
}

// TestFragmentStateIsolation makes sure no field of the key can be ignored
// when matching a later fragment to its datagram.
func TestFragmentStateIsolation(t *testing.T) {
	cache := newFragmentCache()
	base := fragmentKey{
		src: mustAddr(clientNsIP), dst: mustAddr(serverVIP),
		id: 42, version: 4, proto: iputils.ProtoUDP,
	}
	cache.remember(base, fragmentTranslation{newSrc: mustAddr(clientInstIP), newDst: mustAddr(serverNsIP)})

	for name, key := range map[string]fragmentKey{
		"different source":   {src: mustAddr(otherNsIP), dst: base.dst, id: 42, version: 4, proto: iputils.ProtoUDP},
		"different dest":     {src: base.src, dst: mustAddr(otherVIP), id: 42, version: 4, proto: iputils.ProtoUDP},
		"different protocol": {src: base.src, dst: base.dst, id: 42, version: 4, proto: iputils.ProtoTCP},
		"different family":   {src: base.src, dst: base.dst, id: 42, version: 6, proto: iputils.ProtoUDP},
		"different id":       {src: base.src, dst: base.dst, id: 43, version: 4, proto: iputils.ProtoUDP},
	} {
		if _, found := cache.lookup(key); found {
			t.Errorf("%s: reused another datagram's fragment translation", name)
		}
	}

	if _, found := cache.lookup(base); !found {
		t.Error("the datagram's own fragment state went missing")
	}
}

func TestFragmentStateExpires(t *testing.T) {
	cache := newFragmentCache()
	key := fragmentKey{src: mustAddr(clientNsIP), dst: mustAddr(serverVIP), id: 7, version: 4, proto: iputils.ProtoUDP}
	cache.remember(key, fragmentTranslation{newSrc: mustAddr(clientInstIP), newDst: mustAddr(serverNsIP)})

	// Age the entry out without waiting on the wall clock.
	cache.mu.Lock()
	entry := cache.entries[key]
	entry.expires = clock.Unix() - 1
	cache.entries[key] = entry
	cache.mu.Unlock()

	if _, found := cache.lookup(key); found {
		t.Error("expired fragment state was still returned")
	}
	cache.mu.Lock()
	remaining := len(cache.entries)
	cache.mu.Unlock()
	if remaining != 0 {
		t.Errorf("expired fragment state was not released (%d entries left)", remaining)
	}
}

func TestFragmentCacheBounded(t *testing.T) {
	cache := newFragmentCache()
	for i := 0; i < maxFragmentEntries*2; i++ {
		cache.remember(fragmentKey{
			src: mustAddr(clientNsIP), dst: mustAddr(serverVIP),
			id: uint32(i), version: 4, proto: iputils.ProtoUDP,
		}, fragmentTranslation{newSrc: mustAddr(clientInstIP), newDst: mustAddr(serverNsIP)})
	}
	cache.mu.Lock()
	size := len(cache.entries)
	cache.mu.Unlock()
	if size > maxFragmentEntries {
		t.Errorf("fragment cache grew to %d entries; cap is %d", size, maxFragmentEntries)
	}
}

// TestUnknownLaterFragmentDropped: without the first fragment there's no way
// to translate this one consistently, so it must be dropped untouched.
func TestUnknownLaterFragmentDropped(t *testing.T) {
	dp := getFakeDatapath()
	wire := buildUDPv4(t, clientNsIP, serverVIP, 40000, 53, largePayload(2000))
	_, later := fragmentIPv4(t, wire, 1024)

	pkt, ok := iputils.Parse(later)
	if !ok {
		t.Fatal("later fragment failed to parse")
	}
	before := append([]byte(nil), pkt.Bytes()...)
	if _, ok := dp.forwardLaterFragment(&pkt); ok {
		t.Error("an unknown later fragment was reported as translated")
	}
	if !bytes.Equal(pkt.Bytes(), before) {
		t.Error("an unknown later fragment was translated")
	}
}

// TestHandleOutgoingForwardsWholeDatagram runs both fragments through a real
// socket and checks they both land on the same node, translated the same way.
func TestHandleOutgoingForwardsWholeDatagram(t *testing.T) {
	tunnel, listener := loopbackTunnel(t)

	wire := buildUDPv4(t, clientNsIP, serverVIP, 40000, 53, largePayload(2000))
	first, later := fragmentIPv4(t, wire, 1024)
	laterPayloadBefore := append([]byte(nil), later[20:]...)

	tunnel.Emit(tunnel.dp.Handle(Outgoing, first))
	tunnel.Emit(tunnel.dp.Handle(Outgoing, later))

	gotFirst := parseTestPacket(t, readForwarded(t, listener))
	if gotFirst.SrcIP() != mustAddr(clientInstIP) || gotFirst.DstIP() != mustAddr(serverNsIP) {
		t.Fatalf("first fragment forwarded as %s -> %s; want %s -> %s",
			gotFirst.SrcIP(), gotFirst.DstIP(), clientInstIP, serverNsIP)
	}

	gotLater, ok := iputils.Parse(readForwarded(t, listener))
	if !ok {
		t.Fatal("forwarded later fragment failed to parse")
	}
	if gotLater.SrcIP() != gotFirst.SrcIP() || gotLater.DstIP() != gotFirst.DstIP() {
		t.Errorf("later fragment forwarded as %s -> %s; want it to match the first fragment %s -> %s",
			gotLater.SrcIP(), gotLater.DstIP(), gotFirst.SrcIP(), gotFirst.DstIP())
	}
	if got := ipv4HeaderChecksum(gotLater.Bytes()[:20]); got != 0 {
		t.Errorf("forwarded later fragment header checksum does not verify (residual %#04x)", got)
	}
	if !bytes.Equal(gotLater.Bytes()[20:], laterPayloadBefore) {
		t.Error("forwarded later fragment payload was modified")
	}
}
