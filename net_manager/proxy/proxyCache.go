package proxy

import (
	"NetManager/TableEntryCache"
	"NetManager/clock"
	"NetManager/events"
	"NetManager/logger"
	"NetManager/resolver"
	"net/netip"
	"sync"
	"time"
)

// ConversionEntry is one translated flow, identified by the full 5-tuple in
// both directions: destination port alone collides as soon as one local
// socket talks to two different Service VIPs on the same port (the common UDP
// shape), reverse-translating a reply for one VIP as though it belonged to
// the other.
type ConversionEntry struct {
	srcip         netip.Addr
	dstip         netip.Addr
	dstServiceIp  netip.Addr
	srcInstanceIp netip.Addr
	// dstInstanceIp is the address replies actually arrive from (not dstip:
	// the remote node's outgoingProxy sources them from its own instance IP).
	// Zero means "matches any remote", used when the entry has no
	// InstanceNumber ServiceIP to predict a reply source from.
	dstInstanceIp netip.Addr
	srcport       int
	dstport       int
	protocol      uint8
	// dstNode/dstNodePort cache which node dstip actually lives on, so the
	// packet path doesn't need a second table lookup by namespace IP.
	dstNode     netip.Addr
	dstNodePort int
	// activity is the destination job's shared last-used stamp, held here so
	// a warm flow can keep the job's MQTT interest alive without looking its
	// table entry up again. Nil for a flow installed from an entry that never
	// went through TableManager.Add.
	activity *events.Activity
	// routeGen is the translation table generation this route was chosen
	// from. While it still matches, the route is known to be current and the
	// per-replica revalidation scan can be skipped entirely.
	routeGen uint64
	lastUsed int64
}

// touch records that the entry was just used, for the eviction sweep. The
// write is skipped when the stamp already reads the current second: the
// coarse clock only moves at 1Hz, so all but the first packet of each second
// would be storing the value that is already there, dirtying the entry's
// cache line for nothing. Both packet loops run this on entries in the same
// shard, so the line they leave clean is one the other core can keep shared.
func (e *ConversionEntry) touch() {
	if now := clock.Unix(); e.lastUsed != now {
		e.lastUsed = now
	}
}

// conversionBucket holds every flow whose *local* port is this bucket's index.
// Both directions key on the local port (outgoing on its source port, ingoing
// on the reply's destination port), so one direct index serves both without
// hashing anything on the packet path.
type conversionBucket struct {
	entries []ConversionEntry
}

const (
	// maxFlowsPerPort bounds one local port's flow list. A single socket
	// talking to more distinct endpoints than this is possible but unusual;
	// past the cap the least recently used flow is recycled.
	maxFlowsPerPort = 64
	// cacheShards is the number of striped locks over the bucket array.
	// Sharding keeps eviction from taking a lock that covers every flow on
	// the node while the datapath is trying to translate.
	cacheShards = 256
)

type ProxyCache struct {
	// One bucket per local port number. Higher mem usage but lower cpu usage.
	cache []conversionBucket
	locks [cacheShards]sync.Mutex
	frags *fragmentCache
}

func NewProxyCache() *ProxyCache {
	cache := &ProxyCache{
		cache: make([]conversionBucket, 65536),
		frags: newFragmentCache(),
	}
	cache.runEvictionJob(30*time.Second, 1*time.Minute)
	return cache
}

func shardOf(localPort int) int { return localPort & (cacheShards - 1) }

// runEvictionJob starts a goroutine that periodically evicts old cache entries.
func (cache *ProxyCache) runEvictionJob(interval time.Duration, timeout time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			cache.evictOldEntries(timeout)
			cache.frags.evictExpired()
		}
	}()
}

// evictOldEntries removes flows that have not been used for the duration of
// the timeout. It walks one shard at a time so it never holds a lock covering
// the whole table.
func (cache *ProxyCache) evictOldEntries(timeout time.Duration) {
	now := clock.Unix()
	timeoutSeconds := int64(timeout.Seconds())
	evictedCount := 0

	for shard := range cache.locks {
		cache.locks[shard].Lock()
		for port := shard; port < len(cache.cache); port += cacheShards {
			bucket := &cache.cache[port]
			kept := bucket.entries[:0]
			for _, entry := range bucket.entries {
				if now-entry.lastUsed > timeoutSeconds {
					evictedCount++
					continue
				}
				kept = append(kept, entry)
			}
			if len(kept) == 0 {
				bucket.entries = nil
			} else {
				bucket.entries = kept
			}
		}
		cache.locks[shard].Unlock()
	}

	if evictedCount > 0 {
		logger.InfoLogger().Printf("Evicted %d entries from cache", evictedCount)
	}
}

// Route is what the datapath needs from a cached flow: how to rewrite the
// packet, and where to send it.
type Route struct {
	SrcInstanceIP netip.Addr
	DstIP         netip.Addr
	DstNode       netip.Addr
	DstNodePort   int
}

// ReverseRoute is the equivalent for a reply being translated back.
type ReverseRoute struct {
	DstServiceIP netip.Addr
	SrcIP        netip.Addr
}

// FlowKey identifies one flow: the full 5-tuple, as seen leaving the local
// namespace. See ConversionEntry for why the full tuple, not just a port, is
// what identifies a flow.
//
// The source instance IP is deliberately not part of this: it is a function
// of SrcIP under a given translation table generation, so including it would
// force the datapath to resolve it before it could even ask whether the flow
// is cached. A generation bump that changes it is caught by Revalidate
// instead, which refreshes the flow rather than rerouting it.
type FlowKey struct {
	Protocol            uint8
	SrcIP, DstServiceIP netip.Addr
	SrcPort, DstPort    int
}

// matchesFlow reports whether e is the flow that key identifies.
func (e *ConversionEntry) matchesFlow(key *FlowKey) bool {
	return e.protocol == key.Protocol &&
		e.dstport == key.DstPort &&
		e.dstServiceIp == key.DstServiceIP &&
		e.srcip == key.SrcIP
}

// Lookup resolves the cached route for key, but only when that route was
// chosen under gen, meaning the translation table has not been rebuilt
// since, so the route is known current. This is the steady-state packet path,
// and the point of the generation check is that a hit here needs no table
// access at all: not the Service IP index, not the namespace IP index, just
// this one bucket. A miss, or a hit under an older generation, sends the
// caller to Revalidate.
//
// key and route are pointers rather than values because both are large
// enough (four netip.Addr between them) that copying them in and out costs
// more than the bucket scan they bracket.
func (cache *ProxyCache) Lookup(key *FlowKey, gen uint64, route *Route) bool {
	shard := shardOf(key.SrcPort)
	cache.locks[shard].Lock()

	for i := range cache.cache[key.SrcPort].entries {
		entry := &cache.cache[key.SrcPort].entries[i]
		if !entry.matchesFlow(key) {
			continue
		}
		if entry.routeGen != gen || entry.dstport < 1 {
			// The flow is known but its route was picked under a table this
			// one has moved past; Revalidate has to check it before reuse.
			break
		}
		entry.use(route)
		cache.locks[shard].Unlock()
		return true
	}

	cache.locks[shard].Unlock()
	return false
}

// use marks the entry as just used, both for eviction and for the
// destination job's MQTT interest, and writes its route to out. Caller must
// hold the entry's shard lock.
func (e *ConversionEntry) use(out *Route) {
	e.touch()
	if e.activity != nil {
		e.activity.Touch()
	}
	out.SrcInstanceIP = e.srcInstanceIp
	out.DstIP = e.dstip
	out.DstNode = e.dstNode
	out.DstNodePort = e.dstNodePort
}

// Revalidate checks a known flow's route against the current table and, if
// the instance it is pinned to is still there, retags it with the current
// generation so later packets take Lookup's fast path again. It deliberately
// refreshes rather than re-routes: an established connection has to keep the
// replica it was pinned to for as long as that replica exists. srcInstanceIP
// is re-resolved by the caller and refreshed here, since a table rebuild can
// have moved it; version selects the address family the refreshed reply
// source is read in.
//
// ok is false when there is no cached flow, or its instance is gone; either
// way the caller has to choose a new route and Install it.
func (cache *ProxyCache) Revalidate(key FlowKey, srcInstanceIP netip.Addr, version uint8, lookup resolver.ServiceLookup) (Route, bool) {
	shard := shardOf(key.SrcPort)
	cache.locks[shard].Lock()
	defer cache.locks[shard].Unlock()

	for i := range cache.cache[key.SrcPort].entries {
		entry := &cache.cache[key.SrcPort].entries[i]
		if !entry.matchesFlow(&key) {
			continue
		}
		if entry.dstport < 1 {
			return Route{}, false
		}
		src := TableEntryCache.MatchRoute(entry.dstip, entry.dstNode, entry.dstNodePort, lookup.Entries)
		if src == nil {
			return Route{}, false
		}
		entry.routeGen = lookup.Generation
		entry.srcInstanceIp = srcInstanceIP
		// Refreshed from the entry the route survived against, not carried
		// over: a job removed and re-added between the two generations gets a
		// fresh activity stamp, and touching the retired one would let the
		// job's MQTT interest self-destruct under a flow that is still live.
		entry.dstInstanceIp = TableEntryCache.InstanceAddrsOf(src).For(version)
		entry.activity = src.Activity()

		var route Route
		entry.use(&route)
		return route, true
	}
	return Route{}, false
}

// Install records a freshly chosen route for key, tagged with the translation
// table generation it was chosen from. src is the entry the route was chosen
// from: both the address replies will arrive from and the destination job's
// activity stamp are read off it here, so the packet path never has to go
// back to the table for them.
func (cache *ProxyCache) Install(key FlowKey, r Route, src *TableEntryCache.TableEntry, version uint8, gen uint64) {
	cache.Add(ConversionEntry{
		srcip:         key.SrcIP,
		dstip:         r.DstIP,
		dstServiceIp:  key.DstServiceIP,
		srcInstanceIp: r.SrcInstanceIP,
		dstInstanceIp: TableEntryCache.InstanceAddrsOf(src).For(version),
		srcport:       key.SrcPort,
		dstport:       key.DstPort,
		protocol:      key.Protocol,
		dstNode:       r.DstNode,
		dstNodePort:   r.DstNodePort,
		activity:      src.Activity(),
		routeGen:      gen,
	})
}

// Reverse resolves the flow a reply belongs to, so it can be
// reverse-translated. The arguments are read off the reply packet: it is
// addressed to the local namespace IP and local port the flow originated from,
// and sourced from the remote instance IP and port it was sent to.
func (cache *ProxyCache) Reverse(protocol uint8, localNsIP netip.Addr, localPort int,
	remoteIP netip.Addr, remotePort int) (ReverseRoute, bool) {
	shard := shardOf(localPort)
	cache.locks[shard].Lock()
	defer cache.locks[shard].Unlock()

	bucket := &cache.cache[localPort]
	for i := range bucket.entries {
		entry := &bucket.entries[i]
		if entry.protocol != protocol || entry.dstport != remotePort || entry.srcip != localNsIP {
			continue
		}
		// A zero dstInstanceIp means the route had no InstanceNumber
		// ServiceIP to predict the reply source from; fall back to the
		// looser match rather than dropping the flow's replies entirely.
		if entry.dstInstanceIp.IsValid() && entry.dstInstanceIp != remoteIP {
			continue
		}
		entry.touch()
		return ReverseRoute{DstServiceIP: entry.dstServiceIp, SrcIP: entry.srcip}, true
	}
	return ReverseRoute{}, false
}

// sameFlowAs compares the full forward identity of two entries. Anything less,
// notably comparing destination port alone, conflates distinct flows.
func (e *ConversionEntry) sameFlowAs(other *ConversionEntry) bool {
	return e.protocol == other.protocol &&
		e.srcport == other.srcport &&
		e.dstport == other.dstport &&
		e.srcip == other.srcip &&
		e.dstServiceIp == other.dstServiceIp
}

// Add inserts a flow, replacing the entry for the same flow if there is one.
func (cache *ProxyCache) Add(entry ConversionEntry) {
	entry.lastUsed = clock.Unix()

	shard := shardOf(entry.srcport)
	cache.locks[shard].Lock()
	defer cache.locks[shard].Unlock()

	bucket := &cache.cache[entry.srcport]
	for i := range bucket.entries {
		if bucket.entries[i].sameFlowAs(&entry) {
			bucket.entries[i] = entry
			return
		}
	}

	if len(bucket.entries) < maxFlowsPerPort {
		bucket.entries = append(bucket.entries, entry)
		return
	}

	// Bucket full: recycle the least recently used flow.
	oldest := 0
	for i := range bucket.entries {
		if bucket.entries[i].lastUsed < bucket.entries[oldest].lastUsed {
			oldest = i
		}
	}
	bucket.entries[oldest] = entry
}
