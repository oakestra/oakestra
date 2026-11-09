package iputils

import (
	"encoding/binary"
	"net/netip"
	"testing"
)

// sumFields is a deliberately naive reimplementation of RFC 1071's
// ones-complement sum, sharing no code with checksum.go so that a bug in the
// incremental adjustment can't hide behind the same bug in the check.
func sumFields(fields ...[]byte) uint16 {
	var sum uint32
	for _, field := range fields {
		for i := 0; i+1 < len(field); i += 2 {
			sum += uint32(field[i])<<8 | uint32(field[i+1])
		}
	}
	for sum > 0xffff {
		sum = sum&0xffff + sum>>16
	}
	// 0xffff and 0x0000 are both ones-complement zero, and an adjustment is
	// free to land on either; canonicalise so the two don't read as a change.
	if sum == 0xffff {
		return 0
	}
	return uint16(sum)
}

// checksummedAddrs returns the address fields Rewrite mutates, aliasing the
// packet's buffer so a residual can be taken before and after the rewrite.
func checksummedAddrs(buf []byte, version uint8) (src, dst []byte) {
	if version == 4 {
		return buf[12:16], buf[16:20]
	}
	return buf[8:24], buf[24:40]
}

// l4ChecksumField returns the two bytes holding the transport checksum, or
// nil when the packet carries no transport header Rewrite would patch.
func l4ChecksumField(p *Packet) []byte {
	if !p.HasTransport() {
		return nil
	}
	off := int(p.l4Start)
	switch p.protocol {
	case ProtoTCP:
		off += 16
	case ProtoUDP:
		off += 6
	default:
		return nil
	}
	return p.buf[off : off+2]
}

// --- seed builders ---
//
// Random bytes hardly ever land on a well-formed header, let alone one with
// options, an extension chain or a truncated transport header. Seeding those
// shapes explicitly is what makes this target worth anything under a plain
// `go test`, which runs the corpus without fuzzing.

func tcpHeader(checksum uint16) []byte {
	h := make([]byte, tcpHeaderMinLen)
	binary.BigEndian.PutUint16(h[0:2], 40000)
	binary.BigEndian.PutUint16(h[2:4], 443)
	h[12] = 0x50 // data offset: 5 words, no options
	binary.BigEndian.PutUint16(h[16:18], checksum)
	return h
}

func udpHeader(checksum uint16) []byte {
	h := make([]byte, udpHeaderLen)
	binary.BigEndian.PutUint16(h[0:2], 40000)
	binary.BigEndian.PutUint16(h[2:4], 443)
	binary.BigEndian.PutUint16(h[4:6], udpHeaderLen)
	binary.BigEndian.PutUint16(h[6:8], checksum)
	return h
}

// v4Seed builds an IPv4 datagram whose header is words*4 bytes long, so a
// words above 5 leaves room for options and moves the transport header off
// its usual offset.
func v4Seed(words int, protocol uint8, flagsAndOffset uint16, l4 []byte) []byte {
	buf := make([]byte, words*4+len(l4))
	buf[0] = 0x40 | byte(words)
	binary.BigEndian.PutUint16(buf[2:4], uint16(len(buf)))
	binary.BigEndian.PutUint16(buf[6:8], flagsAndOffset)
	buf[9] = protocol
	binary.BigEndian.PutUint16(buf[10:12], 0xb1ff)
	copy(buf[12:16], []byte{10, 19, 1, 1})
	copy(buf[16:20], []byte{10, 30, 0, 1})
	copy(buf[words*4:], l4)
	return buf
}

func v6Seed(nextHeader uint8, ext, l4 []byte) []byte {
	buf := make([]byte, ipv6HeaderLen+len(ext)+len(l4))
	buf[0] = 0x60
	binary.BigEndian.PutUint16(buf[4:6], uint16(len(ext)+len(l4)))
	buf[6] = nextHeader
	copy(buf[8:24], netip.MustParseAddr("fc00::1").AsSlice())
	copy(buf[24:40], netip.MustParseAddr("fdff:2000::ff").AsSlice())
	copy(buf[ipv6HeaderLen:], ext)
	copy(buf[ipv6HeaderLen+len(ext):], l4)
	return buf
}

func FuzzParseAndRewrite(f *testing.F) {
	for _, seed := range [][]byte{
		{},
		v4Seed(5, ProtoTCP, 0, tcpHeader(0x1234)),
		v4Seed(5, ProtoUDP, 0, udpHeader(0x1234)),
		// UDP's two reserved checksums: 0 means "not computed" and must
		// survive untouched, 0xffff is what a real checksum folding to zero
		// is transmitted as.
		v4Seed(5, ProtoUDP, 0, udpHeader(0)),
		v4Seed(5, ProtoUDP, 0, udpHeader(0xffff)),
		// Options push the transport header past its usual offset.
		v4Seed(6, ProtoTCP, 0, tcpHeader(0x1234)),
		// Truncated transport headers: parsed, but with nothing to patch.
		v4Seed(5, ProtoTCP, 0, tcpHeader(0x1234)[:4]),
		v4Seed(5, ProtoUDP, 0, udpHeader(0x1234)[:4]),
		// First and later fragments of a v4 datagram (MF set; offset 0x0100
		// is 2048 bytes in).
		v4Seed(5, ProtoTCP, 0x2000, tcpHeader(0x1234)),
		v4Seed(5, ProtoTCP, 0x0100, tcpHeader(0x1234)),
		v6Seed(ProtoTCP, nil, tcpHeader(0x1234)),
		v6Seed(ProtoUDP, nil, udpHeader(0x1234)),
		// A v6 UDP checksum of zero is a real checksum, not "not computed",
		// so unlike the v4 case it does get adjusted.
		v6Seed(ProtoUDP, nil, udpHeader(0)),
		v6Seed(ProtoTCP, nil, tcpHeader(0x1234)[:4]),
		// Hop-by-hop options, then a fragment header: the extension-header
		// walk has to land the transport offset past both.
		v6Seed(0, []byte{ProtoTCP, 0, 0, 0, 0, 0, 0, 0}, tcpHeader(0x1234)),
		v6Seed(44, []byte{ProtoTCP, 0, 0x00, 0x01, 0, 0, 0, 7}, tcpHeader(0x1234)),
		v6Seed(44, []byte{ProtoTCP, 0, 0x01, 0x01, 0, 0, 0, 7}, tcpHeader(0x1234)),
	} {
		f.Add(seed)
	}

	v4Src := netip.MustParseAddr("10.30.255.253")
	v4Dst := netip.MustParseAddr("10.19.2.12")
	v6Src := netip.MustParseAddr("fdff::fd")
	v6Dst := netip.MustParseAddr("fd00::12")
	f.Fuzz(func(t *testing.T, wire []byte) {
		// Larger buffers are not valid IP datagrams and only waste fuzzing
		// memory when the input is copied for the in-place rewrite.
		if len(wire) > 65535 {
			return
		}
		buf := append([]byte(nil), wire...)
		packet, ok := Parse(buf)
		if !ok {
			return
		}

		version := packet.Version()
		protocol := packet.Protocol()
		fragment := packet.IsFragment()
		firstFragment := packet.IsFirstFragment()
		fragmentID := packet.FragmentID()
		hasTransport := packet.HasTransport()
		var srcPort, dstPort uint16
		if hasTransport {
			srcPort = packet.SrcPort()
			dstPort = packet.DstPort()
		}

		// Checksums over fuzzed bytes are almost never valid, so we can't
		// check that the result verifies. Instead we check the residual
		// (address fields plus checksum, summed together) doesn't change:
		// a correct rewrite cancels the address delta against the checksum
		// delta exactly, whatever the absolute value is. Only fields that
		// actually change need summing; the payload is untouched.
		src, dst := checksummedAddrs(buf, version)
		l4Csum := l4ChecksumField(&packet)
		// An IPv4/UDP checksum of zero means "not computed" and is left
		// alone rather than adjusted, so its residual is expected to move.
		zeroUDPChecksum := version == 4 && protocol == ProtoUDP &&
			l4Csum != nil && l4Csum[0] == 0 && l4Csum[1] == 0
		var ipResidual, l4Residual uint16
		if version == 4 {
			ipResidual = sumFields(src, dst, buf[10:12])
		}
		if l4Csum != nil {
			l4Residual = sumFields(src, dst, l4Csum)
		}

		var newSrc, newDst netip.Addr
		if version == 4 {
			newSrc = v4Src
			newDst = v4Dst
		} else {
			newSrc = v6Src
			newDst = v6Dst
		}
		if !packet.Rewrite(newSrc, newDst) {
			t.Fatal("a successfully parsed packet could not be rewritten")
		}

		if version == 4 {
			if got := sumFields(src, dst, buf[10:12]); got != ipResidual {
				t.Errorf("IPv4 header checksum residual changed from %#04x to %#04x", ipResidual, got)
			}
		}
		switch {
		case l4Csum == nil:
		case zeroUDPChecksum:
			if l4Csum[0] != 0 || l4Csum[1] != 0 {
				t.Error("an IPv4/UDP checksum of zero was adjusted instead of left alone")
			}
		default:
			if got := sumFields(src, dst, l4Csum); got != l4Residual {
				t.Errorf("transport checksum residual changed from %#04x to %#04x", l4Residual, got)
			}
			if protocol == ProtoUDP && l4Csum[0] == 0 && l4Csum[1] == 0 {
				t.Error("rewriting turned a real UDP checksum into the reserved 'not computed' value")
			}
		}

		rewritten, ok := Parse(packet.Bytes())
		if !ok {
			t.Fatal("rewriting made a parsed packet invalid")
		}
		if rewritten.SrcIP() != newSrc || rewritten.DstIP() != newDst {
			t.Error("rewriting produced the wrong source or destination address")
		}
		if rewritten.Version() != version || rewritten.Protocol() != protocol {
			t.Error("rewriting changed the IP version or transport protocol")
		}
		if rewritten.IsFragment() != fragment ||
			rewritten.IsFirstFragment() != firstFragment ||
			rewritten.FragmentID() != fragmentID {
			t.Error("rewriting changed fragment metadata")
		}
		if rewritten.HasTransport() != hasTransport {
			t.Error("rewriting changed transport-header availability")
		} else if hasTransport &&
			(rewritten.SrcPort() != srcPort || rewritten.DstPort() != dstPort) {
			t.Error("rewriting changed transport ports")
		}
	})
}
