package iputils

import (
	"bytes"
	"encoding/binary"
	"math/rand/v2"
	"net"
	"net/netip"
	"testing"

	"github.com/google/gopacket/layers"
)

// The incremental update sums 32 bits at a time and folds the deferred carries
// once at the end, so a mistake in the folding only shows up for address pairs
// that actually produce a carry out of the top half. The fixed-vector
// MatchesFullRecompute tests can't reach those by chance, so sweep random
// address pairs against the same full-recompute reference.

func randAddr4(r *rand.Rand) netip.Addr {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], r.Uint32())
	return netip.AddrFrom4(b)
}

func randAddr16(r *rand.Rand) netip.Addr {
	var b [16]byte
	binary.BigEndian.PutUint64(b[0:8], r.Uint64())
	binary.BigEndian.PutUint64(b[8:16], r.Uint64())
	return netip.AddrFrom16(b)
}

func TestRewriteV4MatchesFullRecomputeOverRandomAddresses(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 2000; i++ {
		src, dst := randAddr4(r), randAddr4(r)
		newSrc, newDst := randAddr4(r), randAddr4(r)

		wire := buildIPv4(t, src.String(), dst.String(),
			&layers.TCP{SrcPort: 40000, DstPort: 80, Seq: 12345, SYN: true, Window: 1024},
			[]byte("payload for the checksum sweep"))

		want := referenceTranslateV4(t, append([]byte(nil), wire...),
			net.IP(newSrc.AsSlice()), net.IP(newDst.AsSlice()))

		got := append([]byte(nil), wire...)
		pkt, ok := Parse(got)
		if !ok || !pkt.HasTransport() {
			t.Fatalf("Parse failed (ok=%v) for %s -> %s", ok, src, dst)
		}
		if !pkt.Rewrite(newSrc, newDst) {
			t.Fatalf("Rewrite returned false for %s -> %s", newSrc, newDst)
		}
		if !bytes.Equal(pkt.Bytes(), want) {
			t.Fatalf("mismatch rewriting %s/%s to %s/%s:\n got  = % x\n want = % x",
				src, dst, newSrc, newDst, pkt.Bytes(), want)
		}
	}
}

func TestRewriteV6MatchesFullRecomputeOverRandomAddresses(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for i := 0; i < 2000; i++ {
		src, dst := randAddr16(r), randAddr16(r)
		newSrc, newDst := randAddr16(r), randAddr16(r)

		wire := buildIPv6(t, src.String(), dst.String(),
			&layers.UDP{SrcPort: 40000, DstPort: 53}, []byte("payload for the checksum sweep"))

		want := referenceTranslateV6(t, append([]byte(nil), wire...),
			net.IP(newSrc.AsSlice()), net.IP(newDst.AsSlice()))

		got := append([]byte(nil), wire...)
		pkt, ok := Parse(got)
		if !ok || !pkt.HasTransport() {
			t.Fatalf("Parse failed (ok=%v) for %s -> %s", ok, src, dst)
		}
		if !pkt.Rewrite(newSrc, newDst) {
			t.Fatalf("Rewrite returned false for %s -> %s", newSrc, newDst)
		}
		if !bytes.Equal(pkt.Bytes(), want) {
			t.Fatalf("mismatch rewriting %s/%s to %s/%s:\n got  = % x\n want = % x",
				src, dst, newSrc, newDst, pkt.Bytes(), want)
		}
	}
}

// foldCarry's unrolled sequence has to converge for every accumulator this
// package can build; the loop it replaced converged by construction.
func TestFoldCarryConverges(t *testing.T) {
	loopFold := func(sum uint64) uint16 {
		for sum>>16 != 0 {
			sum = (sum & 0xffff) + (sum >> 16)
		}
		return uint16(sum)
	}

	// 2^40 comfortably exceeds the widest sum here: 16 complemented 32-bit
	// address words plus a complemented checksum.
	const maxSum = uint64(1) << 40
	r := rand.New(rand.NewPCG(5, 6))
	for i := 0; i < 100000; i++ {
		sum := r.Uint64() % maxSum
		if got, want := foldCarry(sum), loopFold(sum); got != want {
			t.Fatalf("foldCarry(%#x) = %#x, loop reference = %#x", sum, got, want)
		}
	}
	for _, sum := range []uint64{0, 1, 0xffff, 0x10000, 0x1ffff, 0xffffffff, 0x1_0000_0000, maxSum - 1} {
		if got, want := foldCarry(sum), loopFold(sum); got != want {
			t.Fatalf("foldCarry(%#x) = %#x, loop reference = %#x", sum, got, want)
		}
	}
}
