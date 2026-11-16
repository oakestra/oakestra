package proxy

import (
	"NetManager/proxy/iputils"
	"NetManager/resolver"
	"fmt"
	"net/netip"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/ipv4"
)

// These benchmarks exercise the packet-translation hot path exactly as the
// datapath goroutines call it and should show 0 allocs/op.

func benchmarkOutgoing(b *testing.B, wire []byte) {
	dp := getFakeDatapath()
	buf := make([]byte, len(wire))

	// warm the flow cache so the benchmark measures the steady state rather
	// than route selection
	copy(buf, wire)
	pkt, _ := iputils.Parse(buf)
	if _, _, _, ok := dp.outgoingProxy(&pkt); !ok {
		b.Fatal("expected packet to be proxied")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy(buf, wire)
		pkt, ok := iputils.Parse(buf)
		if !ok || !pkt.HasTransport() {
			b.Fatal("parse failed")
		}
		if _, _, _, proxied := dp.outgoingProxy(&pkt); !proxied {
			b.Fatal("expected packet to be proxied")
		}
	}
}

func BenchmarkOutgoingProxyV4(b *testing.B) {
	benchmarkOutgoing(b, buildTestPacketV4(b, clientNsIP, serverVIP, 666, 80))
}

func BenchmarkOutgoingProxyV6(b *testing.B) {
	benchmarkOutgoing(b, buildTestPacketV6(b, clientNsIPv6, serverVIPv6, 666, 80))
}

func BenchmarkIngoingProxyV4(b *testing.B) {
	dp := getFakeDatapath()
	dp.proxycache.Add(ConversionEntry{
		srcip:         mustAddr(clientNsIP),
		dstip:         mustAddr(serverNsIP),
		dstServiceIp:  mustAddr(serverVIP),
		srcInstanceIp: mustAddr(clientInstIP),
		dstInstanceIp: mustAddr(serverInstIP),
		srcport:       666,
		dstport:       80,
		protocol:      iputils.ProtoTCP,
	})
	wire := buildTestPacketV4(b, serverInstIP, clientNsIP, 80, 666)
	buf := make([]byte, len(wire))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy(buf, wire)
		pkt, ok := iputils.Parse(buf)
		if !ok || !pkt.HasTransport() {
			b.Fatal("parse failed")
		}
		if !dp.ingoingProxy(&pkt) {
			b.Fatal("expected packet to match reverse cache entry")
		}
	}
}

// This goes through Tunnel.Emit, the replay path with a fresh scratch
// outgoingBatch per call, not the batched runOutgoingBatch loop the daemon
// actually runs, so its allocations come from that plus the write itself.
// BenchmarkOutgoingBatchGrouping covers the batched path.
func BenchmarkHandleOutgoingLoopback(b *testing.B) {
	tunnel, listener := loopbackTunnel(b)

	// Drain the far end so the socket buffer can't fill and skew the writes.
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 65536)
		for {
			select {
			case <-done:
				return
			default:
			}
			_ = listener.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			if _, _, err := listener.ReadFromUDP(buf); err != nil {
				continue
			}
		}
	}()
	b.Cleanup(func() { close(done) })

	wire := buildTestPacketV4(b, clientNsIP, serverVIP, 666, 80)
	buf := make([]byte, len(wire))

	// Warm on a copy: Handle rewrites the packet in place, and reusing the
	// rewritten one would measure a destination that's no longer proxied.
	copy(buf, wire)
	tunnel.Emit(tunnel.dp.Handle(Outgoing, buf))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy(buf, wire)
		tunnel.Emit(tunnel.dp.Handle(Outgoing, buf))
	}
}

// BenchmarkFlowCacheParallel exercises the striped locks with both datapath
// directions running at once.
func BenchmarkFlowCacheParallel(b *testing.B) {
	dp := getFakeDatapath()
	outgoing := buildTestPacketV4(b, clientNsIP, serverVIP, 40000, 443)
	dp.Handle(Outgoing, append([]byte(nil), outgoing...))
	incoming := buildTestPacketV4(b, serverInstIP, clientNsIP, 443, 40000)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		out := make([]byte, len(outgoing))
		in := make([]byte, len(incoming))
		for pb.Next() {
			copy(out, outgoing)
			pkt, _ := iputils.Parse(out)
			dp.outgoingProxy(&pkt)

			copy(in, incoming)
			reply, _ := iputils.Parse(in)
			dp.ingoingProxy(&reply)
		}
	})
}

// BenchmarkOutgoingProxyReplicas shows a warm cache hit stays cheap as the
// replica count grows, since the generation match skips the revalidation scan.
func BenchmarkOutgoingProxyReplicas(b *testing.B) {
	for _, replicas := range []int{1, 10, 100, 1000} {
		b.Run(fmt.Sprintf("replicas=%d", replicas), func(b *testing.B) {
			dp := fakeDatapathOn(nodeAIP, newFakeEnv(replicatedFixture(b, replicas)...))

			wire := buildTestPacketV4(b, clientNsIP, serverVIP, 666, 80)
			buf := make([]byte, len(wire))
			copy(buf, wire)
			pkt, _ := iputils.Parse(buf)
			if _, _, _, ok := dp.outgoingProxy(&pkt); !ok {
				b.Fatal("expected packet to be proxied")
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				copy(buf, wire)
				pkt, _ := iputils.Parse(buf)
				if _, _, _, ok := dp.outgoingProxy(&pkt); !ok {
					b.Fatal("expected packet to be proxied")
				}
			}
		})
	}
}

// BenchmarkOutgoingProxyRegeneration is the other cache hit: the generation
// has moved on, so each call pays for one revalidation scan.
func BenchmarkOutgoingProxyRegeneration(b *testing.B) {
	for _, replicas := range []int{1, 10, 100, 1000} {
		b.Run(fmt.Sprintf("replicas=%d", replicas), func(b *testing.B) {
			dp := fakeDatapathOn(nodeAIP, newFakeEnv(replicatedFixture(b, replicas)...))

			wire := buildTestPacketV4(b, clientNsIP, serverVIP, 666, 80)
			buf := make([]byte, len(wire))
			copy(buf, wire)
			pkt, _ := iputils.Parse(buf)
			if _, _, _, ok := dp.outgoingProxy(&pkt); !ok {
				b.Fatal("expected packet to be proxied")
			}

			lookup := dp.environment.GetTableEntryByServiceIP(mustAddr(serverVIP))
			key := FlowKey{
				Protocol:     iputils.ProtoTCP,
				SrcIP:        mustAddr(clientNsIP),
				DstServiceIP: mustAddr(serverVIP),
				SrcPort:      666,
				DstPort:      80,
			}
			instanceIP := mustAddr(clientInstIP)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// A fresh, never-tagged generation each iteration forces the
				// revalidation scan every time, not just on the first call.
				stale := resolver.ServiceLookup{Entries: lookup.Entries, Generation: lookup.Generation + 1 + uint64(i)}
				if _, ok := dp.proxycache.Revalidate(key, instanceIP, 4, stale); !ok {
					b.Fatal("expected the route to revalidate")
				}
			}
		})
	}
}

// benchNoopBatchWriter counts calls without copying packets like
// fakeBatchWriter does, since that copy would dominate the allocation count
// this benchmark is meant to isolate.
type benchNoopBatchWriter struct{ calls int }

func (w *benchNoopBatchWriter) WriteBatch(ms []ipv4.Message, flags int) (int, error) {
	w.calls++
	return len(ms), nil
}

// BenchmarkOutgoingBatchGrouping measures grouping one TunDevice read's
// ActionForward packets by destination and issuing one WriteBatch per group,
// with real sends stubbed out. Should show 0 allocs/op once the batch's
// buffers, groups and message scratch have grown to fit (see outgoingBatch).
func BenchmarkOutgoingBatchGrouping(b *testing.B) {
	sock := &fakeTunnelSocket{}
	tunDev := &fakeTunDevice{batchSize: 32}
	tunnel := batchTestTunnel(sock, tunDev)

	tunnel.connectionBuffer[netip.AddrPortFrom(mustAddr(nodeBIP), uint16(tunnelPort))] = fakeConn(b, &benchNoopBatchWriter{})
	tunnel.connectionBuffer[netip.AddrPortFrom(mustAddr(nodeCIP), uint16(tunnelPort))] = fakeConn(b, &benchNoopBatchWriter{})

	var packets [][]byte
	for i := 0; i < 16; i++ {
		packets = append(packets, buildTestPacketV4(b, clientNsIP, serverVIP, 40000+i, 443))
	}
	for i := 0; i < 16; i++ {
		packets = append(packets, buildTestPacketV4(b, clientNsIP, otherVIP, 50000+i, 443))
	}

	batch := newOutgoingBatch(tunDev.BatchSize())

	// fakeTunDevice.ReadBatch drains readQueue by reslicing off the front, so
	// reassigning it each iteration is free; rebuilding it with append every
	// time would measure that rebuild instead.
	readQueue := append([][]byte(nil), packets...)

	// Warm the batch's grouping map, buffer slices and message scratch so the
	// measured loop is the steady state.
	tunDev.readQueue = readQueue
	if err := tunnel.runOutgoingBatch(batch); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tunDev.readQueue = readQueue
		if err := tunnel.runOutgoingBatch(batch); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFlowCacheTwoLoops models the shape the daemon actually runs: one
// outgoing loop and one ingoing loop, contending on the shard that carries a
// flow and its own replies. BenchmarkFlowCacheParallel scales to GOMAXPROCS
// instead, which overstates lock contention the daemon can never reach.
func BenchmarkFlowCacheTwoLoops(b *testing.B) {
	dp := getFakeDatapath()
	outgoing := buildTestPacketV4(b, clientNsIP, serverVIP, 40000, 443)
	dp.Handle(Outgoing, append([]byte(nil), outgoing...))
	incoming := buildTestPacketV4(b, serverInstIP, clientNsIP, 443, 40000)

	b.ReportAllocs()
	b.ResetTimer()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		buf := make([]byte, len(outgoing))
		for i := 0; i < b.N; i++ {
			copy(buf, outgoing)
			pkt, _ := iputils.Parse(buf)
			dp.outgoingProxy(&pkt)
		}
	}()
	go func() {
		defer wg.Done()
		buf := make([]byte, len(incoming))
		for i := 0; i < b.N; i++ {
			copy(buf, incoming)
			pkt, _ := iputils.Parse(buf)
			dp.ingoingProxy(&pkt)
		}
	}()
	wg.Wait()
}
