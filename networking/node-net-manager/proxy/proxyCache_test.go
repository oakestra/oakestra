package proxy

import (
	"NetManager/clock"
	"NetManager/proxy/iputils"
	"net/netip"
	"testing"
	"time"
)

func newTestProxyCache() *ProxyCache {
	return &ProxyCache{
		cache: make([]conversionBucket, 65536),
		frags: newFragmentCache(),
	}
}

func flowCacheTestEntry(id, srcPort int) ConversionEntry {
	octet := byte(id)
	return ConversionEntry{
		srcip:         netip.AddrFrom4([4]byte{10, 19, 1, 1}),
		dstip:         netip.AddrFrom4([4]byte{10, 19, 2, octet}),
		dstServiceIp:  netip.AddrFrom4([4]byte{10, 30, 0, octet}),
		srcInstanceIp: netip.AddrFrom4([4]byte{10, 30, 1, 1}),
		dstInstanceIp: netip.AddrFrom4([4]byte{10, 30, 2, octet}),
		srcport:       srcPort,
		dstport:       443,
		protocol:      iputils.ProtoTCP,
		dstNode:       netip.AddrFrom4([4]byte{10, 0, 0, octet}),
		dstNodePort:   tunnelPort,
		routeGen:      1,
	}
}

func flowKeyOf(entry ConversionEntry) FlowKey {
	return FlowKey{
		Protocol:     entry.protocol,
		SrcIP:        entry.srcip,
		DstServiceIP: entry.dstServiceIp,
		SrcPort:      entry.srcport,
		DstPort:      entry.dstport,
	}
}

// translate runs one outgoing packet through the datapath and returns the
// node it was forwarded to, leaving the flow in the cache.
func translate(t *testing.T, dp *Datapath, wire []byte) string {
	t.Helper()
	pkt := parseTestPacket(t, wire)
	dstNode, _, _, ok := dp.outgoingProxy(&pkt)
	if !ok {
		t.Fatal("packet should have been proxied")
	}
	return dstNode.String()
}

// reverse runs one incoming packet through the datapath and returns the
// source address the local namespace ends up seeing.
func reverse(t *testing.T, dp *Datapath, wire []byte) string {
	t.Helper()
	pkt := parseTestPacket(t, wire)
	if !dp.ingoingProxy(&pkt) {
		t.Fatal("packet should have matched a reverse cache entry")
	}
	return pkt.SrcIP().String()
}

// cachedRoute looks up a flow's cached route the same way outgoingProxy does
// on a cache hit: against the current table generation, so a hit always takes
// the fast path regardless of what else has happened to the table in the
// meantime.
func cachedRoute(t *testing.T, dp *Datapath, protocol uint8, srcIP, dstServiceIP string, srcPort, dstPort int) (Route, bool) {
	t.Helper()
	key := FlowKey{
		Protocol:     protocol,
		SrcIP:        mustAddr(srcIP),
		DstServiceIP: mustAddr(dstServiceIP),
		SrcPort:      srcPort,
		DstPort:      dstPort,
	}
	var route Route
	ok := dp.proxycache.Lookup(&key, dp.environment.TableGeneration(), &route)
	return route, ok
}

// routeGenOf reads the generation tag off a cached entry directly, since the
// public Route API deliberately doesn't expose it: callers only ever need to
// know whether a cached route is current, not what generation tagged it.
func routeGenOf(t *testing.T, dp *Datapath, protocol uint8, srcIP, srcInstanceIP, dstServiceIP string, srcPort, dstPort int) uint64 {
	t.Helper()
	shard := shardOf(srcPort)
	dp.proxycache.locks[shard].Lock()
	defer dp.proxycache.locks[shard].Unlock()

	bucket := &dp.proxycache.cache[srcPort]
	for i := range bucket.entries {
		entry := &bucket.entries[i]
		if entry.protocol == protocol &&
			entry.dstport == dstPort &&
			entry.dstServiceIp == mustAddr(dstServiceIP) &&
			entry.srcip == mustAddr(srcIP) &&
			entry.srcInstanceIp == mustAddr(srcInstanceIP) {
			return entry.routeGen
		}
	}
	t.Fatal("no cached entry found")
	return 0
}

// TestFlowCacheTwoServiceIPsSameSourcePort checks that two flows from the
// same local socket to different Service VIPs on the same destination port do
// not collide in the same source-port bucket.
func TestFlowCacheTwoServiceIPsSameSourcePort(t *testing.T) {
	dp := getFakeDatapath()

	if node := translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443)); node != nodeBIP {
		t.Fatalf("first flow forwarded to %s; want %s", node, nodeBIP)
	}
	if node := translate(t, dp, buildTestPacketV4(t, clientNsIP, otherVIP, 40000, 443)); node != nodeCIP {
		t.Fatalf("second flow forwarded to %s; want %s", node, nodeCIP)
	}

	// Both mappings must still be there.
	if _, ok := cachedRoute(t, dp, iputils.ProtoTCP, clientNsIP, serverVIP, 40000, 443); !ok {
		t.Error("first flow's cache entry was destroyed by the second insert")
	}
	if _, ok := cachedRoute(t, dp, iputils.ProtoTCP, clientNsIP, otherVIP, 40000, 443); !ok {
		t.Error("second flow's cache entry is missing")
	}
}

// TestFlowCacheReverseDistinguishesRemote is the damaging consequence of the
// same collision: a reply belonging to one Service VIP must never be
// reverse-translated as though it belonged to another.
func TestFlowCacheReverseDistinguishesRemote(t *testing.T) {
	dp := getFakeDatapath()

	translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))
	translate(t, dp, buildTestPacketV4(t, clientNsIP, otherVIP, 40000, 443))

	// Replies arrive from each target's instance IP (see TestRoundTrip).
	if got := reverse(t, dp, buildTestPacketV4(t, serverInstIP, clientNsIP, 443, 40000)); got != serverVIP {
		t.Errorf("reply from %s reversed to %s; want %s", serverInstIP, got, serverVIP)
	}
	if got := reverse(t, dp, buildTestPacketV4(t, otherInstIP, clientNsIP, 443, 40000)); got != otherVIP {
		t.Errorf("reply from %s reversed to %s; want %s", otherInstIP, got, otherVIP)
	}
}

// TestFlowCacheSeparatesProtocols covers TCP and UDP flows that are otherwise
// identical: same addresses, same ports. They are different flows.
func TestFlowCacheSeparatesProtocols(t *testing.T) {
	dp := getFakeDatapath()

	translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))
	translate(t, dp, buildUDPv4(t, clientNsIP, serverVIP, 40000, 443, []byte("hello")))

	if _, ok := cachedRoute(t, dp, iputils.ProtoTCP, clientNsIP, serverVIP, 40000, 443); !ok {
		t.Fatal("TCP flow's cache entry was destroyed by the UDP insert")
	}
	if _, ok := cachedRoute(t, dp, iputils.ProtoUDP, clientNsIP, serverVIP, 40000, 443); !ok {
		t.Fatal("UDP flow's cache entry is missing")
	}

	// A reply must not cross over between them either.
	if _, found := dp.proxycache.Reverse(iputils.ProtoTCP,
		mustAddr(clientNsIP), 40000, mustAddr(serverInstIP), 443); !found {
		t.Error("no reverse mapping for the TCP flow")
	}
	if _, found := dp.proxycache.Reverse(101,
		mustAddr(clientNsIP), 40000, mustAddr(serverInstIP), 443); found {
		t.Error("a reply for an unrelated protocol matched a cached flow")
	}
}

// TestFlowCacheMultipleUDPDestinations is the ordinary UDP shape: one local
// socket talking to several Service VIPs at once.
func TestFlowCacheMultipleUDPDestinations(t *testing.T) {
	dp := getFakeDatapath()

	vips := []struct{ vip, node string }{
		{serverVIP, nodeBIP},
		{otherVIP, nodeCIP},
		{clientVIP, nodeAIP},
	}
	for _, v := range vips {
		translate(t, dp, buildUDPv4(t, clientNsIP, v.vip, 40000, 53, []byte("q")))
	}
	for _, v := range vips {
		r, ok := cachedRoute(t, dp, iputils.ProtoUDP, clientNsIP, v.vip, 40000, 53)
		if !ok {
			t.Errorf("lost the cache entry for %s", v.vip)
			continue
		}
		if r.DstNode.String() != v.node {
			t.Errorf("%s cached node = %s; want %s", v.vip, r.DstNode, v.node)
		}
	}
}

func TestFlowCacheEvictsOnlyIdleEntries(t *testing.T) {
	cache := newTestProxyCache()
	idle := flowCacheTestEntry(1, 40000)
	active := flowCacheTestEntry(2, 40001)
	cache.Add(idle)
	cache.Add(active)

	shard := shardOf(idle.srcport)
	cache.locks[shard].Lock()
	cache.cache[idle.srcport].entries[0].lastUsed = clock.Unix() - 61
	cache.locks[shard].Unlock()

	cache.evictOldEntries(time.Minute)

	if got := len(cache.cache[idle.srcport].entries); got != 0 {
		t.Errorf("idle bucket contains %d entries after eviction; want 0", got)
	}
	if got := len(cache.cache[active.srcport].entries); got != 1 {
		t.Errorf("active bucket contains %d entries after eviction; want 1", got)
	}
}

func TestFlowCacheLookupRefreshesIdleEntry(t *testing.T) {
	cache := newTestProxyCache()
	entry := flowCacheTestEntry(1, 40000)
	cache.Add(entry)

	shard := shardOf(entry.srcport)
	cache.locks[shard].Lock()
	cache.cache[entry.srcport].entries[0].lastUsed = clock.Unix() - 61
	cache.locks[shard].Unlock()

	key := flowKeyOf(entry)
	var route Route
	if !cache.Lookup(&key, entry.routeGen, &route) {
		t.Fatal("expected the existing flow to be found")
	}
	cache.evictOldEntries(time.Minute)

	if got := len(cache.cache[entry.srcport].entries); got != 1 {
		t.Errorf("recently used bucket contains %d entries after eviction; want 1", got)
	}
}

func TestFlowCacheCapacityRecyclesLeastRecentlyUsed(t *testing.T) {
	cache := newTestProxyCache()
	const srcPort = 40000
	for id := 1; id <= maxFlowsPerPort; id++ {
		cache.Add(flowCacheTestEntry(id, srcPort))
	}

	// Age a flow in the middle of the bucket, not the first one: recycling
	// slot 0 blindly would otherwise look like a correct LRU choice.
	const stale = maxFlowsPerPort / 2
	shard := shardOf(srcPort)
	cache.locks[shard].Lock()
	cache.cache[srcPort].entries[stale].lastUsed = clock.Unix() - 61
	cache.locks[shard].Unlock()

	replacement := flowCacheTestEntry(maxFlowsPerPort+1, srcPort)
	cache.Add(replacement)

	bucket := cache.cache[srcPort].entries
	if got := len(bucket); got != maxFlowsPerPort {
		t.Fatalf("full bucket contains %d entries; want %d", got, maxFlowsPerPort)
	}

	oldest := flowKeyOf(flowCacheTestEntry(stale+1, srcPort))
	replacementKey := flowKeyOf(replacement)
	foundOldest, foundReplacement := false, false
	for i := range bucket {
		foundOldest = foundOldest || bucket[i].matchesFlow(&oldest)
		foundReplacement = foundReplacement || bucket[i].matchesFlow(&replacementKey)
	}
	if foundOldest {
		t.Error("least recently used flow remained in a full bucket")
	}
	if !foundReplacement {
		t.Error("new flow was not inserted into the full bucket")
	}
}
