package proxy

import (
	"NetManager/events"
	"NetManager/proxy/iputils"
	"sync"
	"testing"
)

// cachedEntry reaches into a flow's cached entry so the tests below can assert
// on the state a warm flow carries instead of re-deriving it from the table.
func cachedEntry(t *testing.T, dp *Datapath, srcPort int) *ConversionEntry {
	t.Helper()
	shard := shardOf(srcPort)
	dp.proxycache.locks[shard].Lock()
	defer dp.proxycache.locks[shard].Unlock()

	entries := dp.proxycache.cache[srcPort].entries
	if len(entries) != 1 {
		t.Fatalf("expected exactly one cached flow on port %d, got %d", srcPort, len(entries))
	}
	entry := entries[0]
	return &entry
}

// TestWarmFlowSkipsTheTable is the property the whole fast path rests on: once
// a flow is cached and the table has not been rebuilt, translating a packet
// must not consult the translation table at all.
func TestWarmFlowSkipsTheTable(t *testing.T) {
	dp := getFakeDatapath()
	environment := dp.environment.(*FakeEnv)

	translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))
	if environment.tableLookups == 0 {
		t.Fatal("the first packet of a flow must resolve against the table")
	}

	warm := environment.tableLookups
	for i := 0; i < 5; i++ {
		translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))
	}
	if environment.tableLookups != warm {
		t.Errorf("warm flow made %d table lookups; want none",
			environment.tableLookups-warm)
	}
}

// TestWarmFlowKeepsJobInterestAlive covers the consequence of skipping the table: the
// table lookup is what used to keep the destination job's MQTT interest from
// self-destructing. The cached flow has to carry that job's activity stamp
// itself, or a busy long-lived flow would have its own route torn out from
// under it.
func TestWarmFlowKeepsJobInterestAlive(t *testing.T) {
	dp := getFakeDatapath()
	translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))

	entry := cachedEntry(t, dp, 40000)
	if want := events.GetOrCreate(fixtureEntries[1].JobName); entry.activity != want {
		t.Fatalf("cached flow carries activity %p for the destination job; want the job's shared stamp %p",
			entry.activity, want)
	}
}

// TestRevalidatedFlowAdoptsTheNewJobStamp covers a job that is removed and
// redeployed onto the same node and namespace IP gets a fresh activity stamp,
// and its old one is never polled again. A revalidated flow that kept touching
// the retired stamp would let the redeployed job's interest self-destruct
// while the flow is still running.
func TestRevalidatedFlowAdoptsTheNewJobStamp(t *testing.T) {
	dp := getFakeDatapath()
	environment := dp.environment.(*FakeEnv)
	job := fixtureEntries[1].JobName

	translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))
	retired := cachedEntry(t, dp, 40000).activity

	// Model the redeploy: the job's stamp is dropped, then the identical route
	// comes back, so revalidation finds the instance still there.
	events.Delete(job)
	environment.replaceJob(t, job, fixtureEntries[1])

	translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))

	entry := cachedEntry(t, dp, 40000)
	if entry.activity == retired {
		t.Error("revalidated flow still touches the retired activity stamp")
	}
	if want := events.GetOrCreate(job); entry.activity != want {
		t.Errorf("revalidated flow carries activity %p; want the redeployed job's stamp %p",
			entry.activity, want)
	}
}

// TestRevalidatedFlowRefreshesTheReplySource checks that a revalidated flow
// picks up its reply source afresh. Replies are matched against the address
// the destination instance sources them from, so a redeploy that changes that
// address would otherwise leave every reply on a still-valid route
// unmatched.
func TestRevalidatedFlowRefreshesTheReplySource(t *testing.T) {
	dp := getFakeDatapath()
	environment := dp.environment.(*FakeEnv)
	job := fixtureEntries[1].JobName

	translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))

	// Same node, same namespace IP, different instance address.
	const newInstIP = "10.30.255.240"
	moved := tableEntry("serverapp", nodeBIP, serverNsIP, serverNsIPv6,
		serverVIP, serverVIPv6, newInstIP, serverInstIPv6)
	environment.replaceJob(t, job, moved)

	translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))

	if got := cachedEntry(t, dp, 40000).dstInstanceIp; got != mustAddr(newInstIP) {
		t.Errorf("revalidated flow expects replies from %s; want the redeployed instance %s", got, newInstIP)
	}
	if src := reverse(t, dp, buildTestPacketV4(t, newInstIP, clientNsIP, 443, 40000)); src != serverVIP {
		t.Errorf("reply from the redeployed instance reverse-translated to %s; want %s", src, serverVIP)
	}
}

// TestFlowSurvivesAnInstanceIPChange covers the source instance IP no longer being part
// of the flow key, so a table refresh that changes it has to refresh the
// cached flow rather than leave it stranded behind a key that no longer
// matches.
func TestFlowSurvivesAnInstanceIPChange(t *testing.T) {
	dp := getFakeDatapath()
	environment := dp.environment.(*FakeEnv)

	translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))

	const newClientInstIP = "10.30.255.241"
	moved := tableEntry("clientapp", nodeAIP, clientNsIP, clientNsIPv6,
		clientVIP, clientVIPv6, newClientInstIP, clientInstIPv6)
	environment.replaceJob(t, fixtureEntries[0].JobName, moved)

	wire := buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443)
	pkt := parseTestPacket(t, wire)
	if _, _, _, ok := dp.outgoingProxy(&pkt); !ok {
		t.Fatal("expected the packet to be proxied")
	}
	if got := pkt.SrcIP().String(); got != newClientInstIP {
		t.Errorf("packet sourced from %s; want the refreshed instance IP %s", got, newClientInstIP)
	}
	if entries := len(dp.proxycache.cache[40000].entries); entries != 1 {
		t.Errorf("instance IP change left %d cached flows on the port; want the one refreshed flow", entries)
	}
}

// TestRevalidationKeepsThePinnedReplica checks that a table rebuild leaving a
// flow's replica in place does not reroute that flow: an established
// connection moved to another replica mid-stream is a broken connection.
func TestRevalidationKeepsThePinnedReplica(t *testing.T) {
	dp := fakeDatapathOn(nodeAIP, newFakeEnv(replicatedFixture(t, 8)...))
	environment := dp.environment.(*FakeEnv)

	translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))
	pinned := cachedEntry(t, dp, 40000).dstip

	// Rebuild the table with the same entries: every replica is still there,
	// so the flow has no reason to move.
	environment.replaceJob(t, fixtureEntries[0].JobName, fixtureEntries[0])

	translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))

	if got := cachedEntry(t, dp, 40000).dstip; got != pinned {
		t.Errorf("flow rerouted to %s across a table rebuild; want the pinned replica %s", got, pinned)
	}
}

// TestConcurrentDirectionsShareAFlow runs both packet loops against the same
// cached flow the way the daemon does: a flow and its own replies land in the
// same shard, since both directions key on the local port. Worth running under
// -race: both directions touch the entry's idle stamp.
func TestConcurrentDirectionsShareAFlow(t *testing.T) {
	dp := getFakeDatapath()
	translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))

	key := FlowKey{
		Protocol:     iputils.ProtoTCP,
		SrcIP:        mustAddr(clientNsIP),
		DstServiceIP: mustAddr(serverVIP),
		SrcPort:      40000,
		DstPort:      443,
	}
	gen := dp.environment.TableGeneration()

	failures := make(chan string, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		var route Route
		for range 2000 {
			if !dp.proxycache.Lookup(&key, gen, &route) {
				failures <- "forward route disappeared under concurrent use"
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 2000 {
			if _, ok := dp.proxycache.Reverse(iputils.ProtoTCP,
				mustAddr(clientNsIP), 40000, mustAddr(serverInstIP), 443); !ok {
				failures <- "reverse route disappeared under concurrent use"
				return
			}
		}
	}()
	wg.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
}

// TestConcurrentTableRefreshWhileFlowIsBusy combines the two production
// packet loops with repeated table generation changes. The outgoing loop
// must safely revalidate the entry while the incoming loop is matching and
// touching the same entry.
func TestConcurrentTableRefreshWhileFlowIsBusy(t *testing.T) {
	dp := getFakeDatapath()
	environment := dp.environment.(*FakeEnv)
	translate(t, dp, buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443))

	outgoing := buildTestPacketV4(t, clientNsIP, serverVIP, 40000, 443)
	incoming := buildTestPacketV4(t, serverInstIP, clientNsIP, 443, 40000)
	wantNode := mustAddr(nodeBIP)
	wantOutgoingSrc := mustAddr(clientInstIP)
	wantOutgoingDst := mustAddr(serverNsIP)
	wantIncomingSrc := mustAddr(serverVIP)
	wantIncomingDst := mustAddr(clientNsIP)
	forward := make(chan struct{}, 1)
	reverse := make(chan struct{}, 1)
	results := make(chan string, 2)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		buf := make([]byte, len(outgoing))
		for range forward {
			copy(buf, outgoing)
			pkt, ok := iputils.Parse(buf)
			if !ok {
				results <- "outgoing packet stopped parsing"
				continue
			}
			dstNode, dstPort, _, ok := dp.outgoingProxy(&pkt)
			if !ok {
				results <- "forward route disappeared during table refresh"
				continue
			}
			if dstNode != wantNode || dstPort != tunnelPort {
				results <- "forward route changed during unrelated table refresh"
				continue
			}
			if pkt.SrcIP() != wantOutgoingSrc || pkt.DstIP() != wantOutgoingDst {
				results <- "outgoing packet was rewritten to the wrong addresses"
				continue
			}
			results <- ""
		}
	}()
	go func() {
		defer wg.Done()
		buf := make([]byte, len(incoming))
		for range reverse {
			copy(buf, incoming)
			pkt, ok := iputils.Parse(buf)
			if !ok {
				results <- "incoming packet stopped parsing"
				continue
			}
			if !dp.ingoingProxy(&pkt) {
				results <- "reverse route disappeared during table refresh"
				continue
			}
			if pkt.SrcIP() != wantIncomingSrc || pkt.DstIP() != wantIncomingDst {
				results <- "incoming packet was rewritten to the wrong addresses"
				continue
			}
			results <- ""
		}
	}()

	for range 500 {
		// An unrelated route update still bumps the global generation and
		// therefore forces this flow through Revalidate.
		environment.replaceJob(t, fixtureEntries[2].JobName, fixtureEntries[2])
		forward <- struct{}{}
		reverse <- struct{}{}
		for range 2 {
			if failure := <-results; failure != "" {
				t.Error(failure)
			}
		}
	}
	close(forward)
	close(reverse)
	wg.Wait()

	if got, want := routeGenOf(t, dp, iputils.ProtoTCP, clientNsIP, clientInstIP,
		serverVIP, 40000, 443), environment.TableGeneration(); got != want {
		t.Errorf("cached route generation = %d; want current generation %d", got, want)
	}
}
