// Package iputils provides a zero-copy, zero-allocation view over raw IPv4
// and IPv6 datagrams for the proxy's NAT-style translation: it swaps the
// source and destination addresses in place and fixes up the affected
// checksums incrementally (RFC 1624) instead of re-parsing and rebuilding
// the packet with gopacket.
//
// Only addresses are ever rewritten, never ports or payload, which is
// exactly the case RFC 1624 covers in O(1) without touching the payload.
package iputils

import "net/netip"

// Protocol numbers this package understands. Anything else is passed through
// unparsed at the L4 level (HasTransport reports false).
const (
	ProtoTCP = 6
	ProtoUDP = 17
)

const (
	ipv4HeaderMinLen = 20
	ipv6HeaderLen    = 40
	tcpHeaderMinLen  = 20
	udpHeaderLen     = 8
)

// Packet is a zero-copy view over a single raw IP datagram held in buf.
// Every method reads or mutates buf directly; Packet itself never allocates
// and never copies buf. The zero value is not valid; always obtain a Packet
// via Parse.
//
// Fields are ordered widest-first and l4Start is int32 rather than int so
// the struct packs into 40 bytes. Parse returns one of these by value for
// every packet, so any padding is pure copy cost on that path.
type Packet struct {
	buf      []byte
	fragID   uint32
	l4Start  int32 // offset of the L4 (TCP/UDP) header; -1 if none is present here
	version  uint8 // 4 or 6
	protocol uint8 // IPv4 protocol field / IPv6 next-header of the L4 payload
	fragment bool  // this datagram was fragmented (v4: MF or non-zero offset; v6: a fragment header)
}

// Parse decodes buf just far enough to translate it: IP version, header
// length, protocol, and (for an unfragmented packet or the first fragment)
// the L4 header offset. It returns false if buf is too short or malformed.
// buf is retained, not copied, so the caller must not reuse it while the
// returned Packet is in use.
func Parse(buf []byte) (Packet, bool) {
	if len(buf) < 1 {
		return Packet{}, false
	}
	switch buf[0] >> 4 {
	case 4:
		return parseIPv4(buf)
	case 6:
		return parseIPv6(buf)
	default:
		return Packet{}, false
	}
}

func parseIPv4(buf []byte) (Packet, bool) {
	if len(buf) < ipv4HeaderMinLen {
		return Packet{}, false
	}
	ihl := int(buf[0]&0x0f) * 4
	if ihl < ipv4HeaderMinLen || len(buf) < ihl {
		return Packet{}, false
	}

	p := Packet{
		buf:      buf,
		version:  4,
		protocol: buf[9],
		l4Start:  -1,
	}
	// Fragment Offset is the low 13 bits of the flags+offset field, MF (More
	// Fragments) is bit 0x2000. Only the first fragment (offset 0) carries
	// an L4 header; later fragments are raw payload continuation.
	flagsAndOffset := readUint16(buf[6:8])
	fragOffset := flagsAndOffset & 0x1fff
	p.fragment = fragOffset != 0 || flagsAndOffset&0x2000 != 0
	if p.fragment {
		p.fragID = uint32(readUint16(buf[4:6]))
	}
	if fragOffset == 0 {
		p.l4Start = int32(ihl)
	}
	return p, true
}

func parseIPv6(buf []byte) (Packet, bool) {
	if len(buf) < ipv6HeaderLen {
		return Packet{}, false
	}

	nextHeader := buf[6]
	offset := ipv6HeaderLen
	isFragment := false
	firstFragment := true
	var fragID uint32

	// Walk the extension header chain to find the L4 header. Bounded so a
	// malformed or adversarial chain can't spin forever.
walk:
	for range 8 {
		switch nextHeader {
		case 0, 43, 60: // Hop-by-Hop, Routing, Destination Options: length in 8-byte units
			if len(buf) < offset+2 {
				return Packet{}, false
			}
			nh, extLen := buf[offset], (int(buf[offset+1])+1)*8
			if len(buf) < offset+extLen {
				return Packet{}, false
			}
			nextHeader, offset = nh, offset+extLen
		case 51: // Authentication Header: length in 4-byte units, encoded as (len-2)
			if len(buf) < offset+2 {
				return Packet{}, false
			}
			nh, extLen := buf[offset], (int(buf[offset+1])+2)*4
			if len(buf) < offset+extLen {
				return Packet{}, false
			}
			nextHeader, offset = nh, offset+extLen
		case 44: // Fragment header, fixed 8 bytes
			if len(buf) < offset+8 {
				return Packet{}, false
			}
			isFragment = true
			// Fragment Offset is the top 13 bits of bytes offset+2:offset+4,
			// and the Identification is the 32-bit field at offset+4.
			firstFragment = readUint16(buf[offset+2:offset+4])>>3 == 0
			fragID = readUint32(buf[offset+4 : offset+8])
			nextHeader, offset = buf[offset], offset+8
		default:
			break walk
		}
	}

	p := Packet{
		buf:      buf,
		version:  6,
		protocol: nextHeader,
		l4Start:  -1,
		fragment: isFragment,
		fragID:   fragID,
	}
	if !isFragment || firstFragment {
		p.l4Start = int32(offset)
	}
	return p, true
}

// Version returns 4 or 6.
func (p Packet) Version() uint8 { return p.version }

// Protocol returns the IPv4 protocol field / IPv6 next-header value of the
// L4 payload (e.g. ProtoTCP, ProtoUDP).
func (p Packet) Protocol() uint8 { return p.protocol }

// HasTransport reports whether a TCP or UDP header is present at l4Start and
// fully within the buffer. False for non-TCP/UDP protocols and for
// non-first IP fragments, which carry no L4 header at all.
func (p Packet) HasTransport() bool {
	if p.l4Start < 0 {
		return false
	}
	switch p.protocol {
	case ProtoTCP:
		return len(p.buf) >= int(p.l4Start)+tcpHeaderMinLen
	case ProtoUDP:
		return len(p.buf) >= int(p.l4Start)+udpHeaderLen
	default:
		return false
	}
}

// IsFragment reports whether this packet is one fragment of a larger
// datagram, including the first one (which looks just like an unfragmented
// packet from its L4 header alone).
func (p Packet) IsFragment() bool { return p.fragment }

// IsFirstFragment reports whether this fragment carries the datagram's
// transport header. Meaningful only when IsFragment is true.
func (p Packet) IsFirstFragment() bool { return p.l4Start >= 0 }

// FragmentID returns the datagram identification shared by every fragment of
// one datagram (16 bits for IPv4, 32 for IPv6). Meaningful only when
// IsFragment is true. It is not unique on its own: fragments must be matched
// on address family, addresses and protocol as well.
func (p Packet) FragmentID() uint32 { return p.fragID }

func (p Packet) SrcIP() netip.Addr {
	if p.version == 4 {
		return netip.AddrFrom4([4]byte(p.buf[12:16]))
	}
	return netip.AddrFrom16([16]byte(p.buf[8:24]))
}

func (p Packet) DstIP() netip.Addr {
	if p.version == 4 {
		return netip.AddrFrom4([4]byte(p.buf[16:20]))
	}
	return netip.AddrFrom16([16]byte(p.buf[24:40]))
}

// SrcPort returns the L4 source port. Only valid when HasTransport is true.
func (p Packet) SrcPort() uint16 {
	return readUint16(p.buf[int(p.l4Start) : int(p.l4Start)+2])
}

// DstPort returns the L4 destination port. Only valid when HasTransport is true.
func (p Packet) DstPort() uint16 {
	return readUint16(p.buf[int(p.l4Start)+2 : int(p.l4Start)+4])
}

// Bytes returns the (possibly Rewrite-mutated) backing buffer.
func (p Packet) Bytes() []byte { return p.buf }

// Rewrite replaces the packet's source and destination IP addresses in
// place and fixes up the affected checksums (the IPv4 header checksum and
// the TCP/UDP checksum, which covers the pseudo-header's addresses) using
// RFC 1624 incremental updates. It never reads or copies the payload.
// Returns false if newSrc/newDst don't match the packet's address family or
// the buffer is too short to be valid.
func (p Packet) Rewrite(newSrc, newDst netip.Addr) bool {
	if p.version == 4 {
		return p.rewriteV4(newSrc, newDst)
	}
	return p.rewriteV6(newSrc, newDst)
}

func (p Packet) rewriteV4(newSrc, newDst netip.Addr) bool {
	if !newSrc.Is4() || !newDst.Is4() || len(p.buf) < ipv4HeaderMinLen {
		return false
	}
	srcBytes, dstBytes := newSrc.As4(), newDst.As4()

	// The old addresses are read straight out of the buffer rather than staged
	// into a scratch array first; they stay in place until the copies below.
	delta := addrDelta4(p.buf[12:16], &srcBytes) + addrDelta4(p.buf[16:20], &dstBytes)

	// IPv4 header checksum, offset 10-11.
	writeUint16(p.buf[10:12], checksumAdjust(readUint16(p.buf[10:12]), delta))

	if p.l4Start >= 0 {
		patchL4Checksum(p.buf, int(p.l4Start), p.protocol, delta, true)
	}

	copy(p.buf[12:16], srcBytes[:])
	copy(p.buf[16:20], dstBytes[:])
	return true
}

func (p Packet) rewriteV6(newSrc, newDst netip.Addr) bool {
	if newSrc.Is4() || newDst.Is4() || len(p.buf) < ipv6HeaderLen {
		return false
	}
	srcBytes, dstBytes := newSrc.As16(), newDst.As16()

	delta := addrDelta16(p.buf[8:24], &srcBytes) + addrDelta16(p.buf[24:40], &dstBytes)

	// IPv6 has no header checksum of its own, only the L4 checksum (which
	// covers the pseudo-header's addresses) needs fixing up.
	if p.l4Start >= 0 {
		patchL4Checksum(p.buf, int(p.l4Start), p.protocol, delta, false)
	}

	copy(p.buf[8:24], srcBytes[:])
	copy(p.buf[24:40], dstBytes[:])
	return true
}

// patchL4Checksum fixes up the TCP/UDP checksum at l4Start for the address
// swap described by delta, via RFC 1624 incremental update. ipv4UDP selects
// IPv4/UDP's special case: a stored checksum of 0x0000 means "not computed"
// and must be left alone. TCP and IPv6/UDP have no such exception; there a
// checksum that folds to 0 is transmitted as the reserved all-ones value
// instead.
func patchL4Checksum(buf []byte, l4Start int, protocol uint8, delta uint64, ipv4UDP bool) {
	switch protocol {
	case ProtoTCP:
		if len(buf) >= l4Start+tcpHeaderMinLen {
			off := l4Start + 16
			csum := readUint16(buf[off : off+2])
			writeUint16(buf[off:off+2], checksumAdjust(csum, delta))
		}
	case ProtoUDP:
		if len(buf) >= l4Start+udpHeaderLen {
			off := l4Start + 6
			csum := readUint16(buf[off : off+2])
			if ipv4UDP && csum == 0 {
				return
			}
			newCsum := checksumAdjust(csum, delta)
			if newCsum == 0 {
				newCsum = 0xffff
			}
			writeUint16(buf[off:off+2], newCsum)
		}
	}
}
