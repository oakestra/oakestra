package iputils

import "encoding/binary"

// Internet checksums (RFC 1071/1624) are one's-complement 16-bit sums.
// Translating a packet only ever rewrites the source/destination address
// fields, never the payload, so we don't need to walk the payload and
// recompute the checksum from scratch. RFC 1624 gives an O(1) way to update
// an existing checksum for a field replacement:
//
//	HC' = ~(~HC + ~m + m')   (all additions are one's-complement, i.e. with
//	                          end-around carry)
//
// This generalizes to replacing several 16-bit words at once: sum ~m_i for
// every old word and m'_i for every new word, then fold once at the end.
//
// The sum below is accumulated 32 bits at a time instead of 16. RFC 1071
// allows deferring the carries, and a 32-bit word's two halves land at
// different bit positions in the accumulator, so folding at the end gives
// the same result as summing the halves separately, at a quarter of the
// loads.

// foldCarry reduces a deferred-carry accumulator to a 16-bit one's-complement
// sum. Unrolled instead of looping: the widest sum this package ever builds
// (16 complemented 32-bit words plus a checksum, under 2^40) always converges
// in exactly this many folds, so a data-dependent loop on the packet path
// would only cost more.
func foldCarry(sum uint64) uint16 {
	sum = (sum >> 32) + (sum & 0xffffffff)
	sum = (sum >> 16) + (sum & 0xffff)
	sum = (sum >> 16) + (sum & 0xffff)
	sum = (sum >> 16) + (sum & 0xffff)
	return uint16(sum)
}

// checksumAdjust applies the RFC 1624 incremental update to checksum (as
// stored on the wire, big-endian). delta is the deferred-carry sum of ~old
// and new over the replaced words, already accumulated by the caller; see
// addrDelta4/addrDelta16.
func checksumAdjust(checksum uint16, delta uint64) uint16 {
	return ^foldCarry(uint64(^checksum) + delta)
}

// addrDelta4 is the delta contributed by replacing one 4-byte address.
func addrDelta4(old []byte, new *[4]byte) uint64 {
	return uint64(^readUint32(old)) + uint64(readUint32(new[:]))
}

// addrDelta16 is the delta contributed by replacing one 16-byte address, read
// as four 32-bit words per side.
func addrDelta16(old []byte, new *[16]byte) uint64 {
	_ = old[15] // one bounds check up front instead of four
	return uint64(^readUint32(old[0:4])) + uint64(^readUint32(old[4:8])) +
		uint64(^readUint32(old[8:12])) + uint64(^readUint32(old[12:16])) +
		uint64(readUint32(new[0:4])) + uint64(readUint32(new[4:8])) +
		uint64(readUint32(new[8:12])) + uint64(readUint32(new[12:16]))
}

func readUint16(b []byte) uint16 {
	return binary.BigEndian.Uint16(b)
}

func writeUint16(b []byte, v uint16) {
	binary.BigEndian.PutUint16(b, v)
}

func readUint32(b []byte) uint32 {
	return binary.BigEndian.Uint32(b)
}
