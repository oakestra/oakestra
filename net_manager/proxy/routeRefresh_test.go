package proxy

import (
	"NetManager/proxy/iputils"
	"NetManager/resolver"
	"testing"
	"time"
)

// A hit is tagged with the table's current generation, which is what lets
// the fast path skip rescanning the service's replicas.
func TestCachedRouteSurvivesUnchangedTable(t *testing.T) {
	dp := getFakeDatapath()
	environment := dp.environment.(*FakeEnv)

	if node := translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443)); node != nodeBIP {
		t.Fatalf("forwarded to %s; want %s", node, nodeBIP)
	}

	_, generation := environment.table.SearchByServiceIP(mustAddr(serverVIP))
	if got := routeGenOf(t, dp, iputils.ProtoTCP, clientNsIP, clientInstIP, serverVIP, 40000, 443); got != generation {
		t.Errorf("cached route tagged with generation %d; table is at %d", got, generation)
	}

	if node := translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443)); node != nodeBIP {
		t.Errorf("second packet forwarded to %s; want the cached route %s", node, nodeBIP)
	}
}

// A route retagged by revalidation must then be trusted on its generation
// alone, without rescanning the table on the next lookup.
func TestCachedRouteRevalidatedOnce(t *testing.T) {
	dp := getFakeDatapath()
	environment := dp.environment.(*FakeEnv)

	translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))

	key := FlowKey{
		Protocol:     iputils.ProtoTCP,
		SrcIP:        mustAddr(clientNsIP),
		DstServiceIP: mustAddr(serverVIP),
		SrcPort:      40000,
		DstPort:      443,
	}

	// Bump the table generation without changing serverapp's route at all.
	environment.replaceJob(t, fixtureEntries[1].JobName, fixtureEntries[1])
	entries, generation := environment.table.SearchByServiceIP(mustAddr(serverVIP))

	var route Route
	if dp.proxycache.Lookup(&key, generation, &route) {
		t.Fatal("a route tagged with an older generation should not be trusted on the fast path")
	}
	lookup := resolver.ServiceLookup{Entries: entries, Generation: generation}
	if _, ok := dp.proxycache.Revalidate(key, mustAddr(clientInstIP), 4, lookup); !ok {
		t.Fatal("a route that is still valid should survive revalidation")
	}
	if got := routeGenOf(t, dp, key.Protocol, clientNsIP, clientInstIP, serverVIP, key.SrcPort, key.DstPort); got != generation {
		t.Fatalf("cached route not retagged after revalidation: generation %d, want %d", got, generation)
	}

	if !dp.proxycache.Lookup(&key, generation, &route) {
		t.Error("a cached route was rescanned (and failed) instead of trusting its generation tag")
	}
}

// A cached flow keyed only on namespace IP would keep tunnelling to the old
// node forever once the instance moves.
func TestCachedRouteFollowsNodeChange(t *testing.T) {
	for _, tc := range []struct {
		name          string
		node          string
		port          int
		wantForwarded string
	}{
		{"node ip changed", nodeCIP, tunnelPort, nodeCIP},
		{"node port changed", nodeBIP, tunnelPort + 1, nodeBIP},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dp := getFakeDatapath()
			environment := dp.environment.(*FakeEnv)

			translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))

			moved := tableEntry("serverapp", tc.node, serverNsIP, serverNsIPv6,
				serverVIP, serverVIPv6, serverInstIP, serverInstIPv6)
			moved.Nodeport = tc.port
			environment.replaceJob(t, moved.JobName, moved)

			pkt := parseTestPacket(t, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))
			node, port, _, ok := dp.outgoingProxy(&pkt)
			if !ok {
				t.Fatal("packet should still have been proxied")
			}
			if node.String() != tc.wantForwarded || port != tc.port {
				t.Errorf("forwarded to %s:%d after the refresh; want %s:%d",
					node, port, tc.wantForwarded, tc.port)
			}
		})
	}
}

func TestCachedRouteDroppedWhenInstanceRemoved(t *testing.T) {
	dp := getFakeDatapath()
	environment := dp.environment.(*FakeEnv)

	translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))
	environment.replaceJob(t, fixtureEntries[1].JobName)

	pkt := parseTestPacket(t, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))
	if _, _, _, ok := dp.outgoingProxy(&pkt); ok {
		t.Error("a flow to a removed instance was still forwarded")
	}
}

func TestIdleTunnelConnectionsEvicted(t *testing.T) {
	tunnel, _ := loopbackTunnel(t)

	tunnel.Emit(tunnel.dp.Handle(Outgoing, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443)))

	tunnel.connectionBufferLock.RLock()
	established := len(tunnel.connectionBuffer)
	tunnel.connectionBufferLock.RUnlock()
	if established != 1 {
		t.Fatalf("%d tunnel connections open; want 1", established)
	}

	// A connection used moments ago is not idle.
	tunnel.evictIdleConnections(time.Hour)
	tunnel.connectionBufferLock.RLock()
	kept := len(tunnel.connectionBuffer)
	tunnel.connectionBufferLock.RUnlock()
	if kept != 1 {
		t.Error("a connection in active use was evicted")
	}

	tunnel.evictIdleConnections(0)
	tunnel.connectionBufferLock.RLock()
	remaining := len(tunnel.connectionBuffer)
	tunnel.connectionBufferLock.RUnlock()
	if remaining != 0 {
		t.Errorf("%d idle tunnel connections survived eviction; want 0", remaining)
	}
}
