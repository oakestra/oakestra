package proxy

import (
	"NetManager/clock"
	"NetManager/proxy/iputils"
	"net/netip"
	"sync"
)

// Only the first fragment of a datagram carries the transport ports the flow
// lookup needs, so later fragments can't be translated independently. Rather
// than reassembling the whole datagram on the packet path, the first
// fragment's translation is remembered here and replayed onto its later
// fragments, which need only their addresses rewritten.
const (
	fragmentTTLSeconds = 5
	maxFragmentEntries = 1024
)

// fragmentKey identifies one datagram in flight. The identification field is
// only unique per source/destination/protocol pair, and IPv4 and IPv6 number
// their fragments independently, so all of that belongs in the key; otherwise
// an unrelated datagram reusing an ID would pick up stale translation state.
type fragmentKey struct {
	src, dst netip.Addr
	id       uint32
	version  uint8
	proto    uint8
}

type fragmentTranslation struct {
	newSrc, newDst netip.Addr
	dstNode        netip.Addr
	dstNodePort    int
	expires        int64
}

type fragmentCache struct {
	mu      sync.Mutex
	entries map[fragmentKey]fragmentTranslation
}

func newFragmentCache() *fragmentCache {
	return &fragmentCache{entries: make(map[fragmentKey]fragmentTranslation)}
}

// keyFor builds the lookup key from a packet's *current* addresses. For a
// first fragment this must be called before Rewrite, so the key matches the
// later fragments, which are never rewritten before lookup.
func keyFor(pkt *iputils.Packet) fragmentKey {
	return fragmentKey{
		src:     pkt.SrcIP(),
		dst:     pkt.DstIP(),
		id:      pkt.FragmentID(),
		version: pkt.Version(),
		proto:   pkt.Protocol(),
	}
}

func (fc *fragmentCache) remember(key fragmentKey, tr fragmentTranslation) {
	tr.expires = clock.Unix() + fragmentTTLSeconds

	fc.mu.Lock()
	defer fc.mu.Unlock()

	if _, exists := fc.entries[key]; !exists && len(fc.entries) >= maxFragmentEntries {
		fc.evictExpiredLocked()
		if len(fc.entries) >= maxFragmentEntries {
			fc.dropEarliestLocked()
		}
	}
	fc.entries[key] = tr
}

func (fc *fragmentCache) lookup(key fragmentKey) (fragmentTranslation, bool) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	tr, ok := fc.entries[key]
	if !ok {
		return fragmentTranslation{}, false
	}
	if clock.Unix() > tr.expires {
		delete(fc.entries, key)
		return fragmentTranslation{}, false
	}
	return tr, true
}

func (fc *fragmentCache) evictExpired() {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.evictExpiredLocked()
}

func (fc *fragmentCache) evictExpiredLocked() {
	now := clock.Unix()
	for key, tr := range fc.entries {
		if now > tr.expires {
			delete(fc.entries, key)
		}
	}
}

// dropEarliestLocked makes room when every entry is still live, which only
// happens under a flood of distinct fragmented datagrams.
func (fc *fragmentCache) dropEarliestLocked() {
	var oldestKey fragmentKey
	var oldest int64
	first := true
	for key, tr := range fc.entries {
		if first || tr.expires < oldest {
			oldestKey, oldest, first = key, tr.expires, false
		}
	}
	if !first {
		delete(fc.entries, oldestKey)
	}
}
