package p2p

import (
	"NetManager/TableEntryCache"
	"net"
	"testing"
	"time"
)

func sampleEntry(job, node, hostIP, sip string) RegistryEntry {
	return RegistryEntry{
		JobName:    job,
		NodeUUID:   node,
		Generation: 0,
		Instances: []InstanceEntry{{
			InstanceNumber: 0,
			Status:         "RUNNING",
			NamespaceIP:    "10.18.0.10",
			HostIP:         hostIP,
			HostPort:       50103,
			ServiceIP: []ServiceIPEntry{
				{Type: "RR", Address: sip},
				{Type: "InstanceNumber", Address: "10.30.1.6"},
			},
		}},
	}
}

func TestRegistryMergeLWW(t *testing.T) {
	r := NewRegistry()
	r.Put(sampleEntry("app.ns.svc.ns", "nodeA", "10.0.0.1", "10.30.1.5"))

	// A stale remote (lower version) must not overwrite.
	stale := sampleEntry("app.ns.svc.ns", "nodeA", "10.0.0.9", "10.30.1.5")
	stale.Version = 0
	r.Merge(mustJSON([]RegistryEntry{stale}))
	got := r.FindByJob("app.ns.svc.ns")
	if len(got) != 1 || got[0].Instances[0].HostIP != "10.0.0.1" {
		t.Fatalf("stale merge overwrote current entry: %+v", got)
	}

	// A newer remote (higher version) wins.
	newer := sampleEntry("app.ns.svc.ns", "nodeA", "10.0.0.2", "10.30.1.5")
	newer.Version = 100
	r.Merge(mustJSON([]RegistryEntry{newer}))
	got = r.FindByJob("app.ns.svc.ns")
	if got[0].Instances[0].HostIP != "10.0.0.2" {
		t.Fatalf("newer merge did not win: %+v", got)
	}
}

func TestTableEntryConversion(t *testing.T) {
	r := NewRegistry()
	r.Put(sampleEntry("app.ns.svc.ns", "nodeA", "10.0.0.1", "10.30.1.5"))
	entries := r.TableEntriesForServiceIP("10.30.1.5")
	if len(entries) != 1 {
		t.Fatalf("got %d table entries; want 1", len(entries))
	}
	e := entries[0]
	if e.Appname != "app" || e.Appns != "ns" || e.Servicename != "svc" || e.Servicenamespace != "ns" {
		t.Errorf("job name split wrong: %+v", e)
	}
	if !e.Nodeip.Equal(net.ParseIP("10.0.0.1")) || e.Nodeport != 50103 {
		t.Errorf("node endpoint wrong: %s:%d", e.Nodeip, e.Nodeport)
	}
}

// TestGossipConvergence starts two real memberlist nodes and verifies a service
// advertised on A is discovered by B after it joins.
func TestGossipConvergence(t *testing.T) {
	key := DeriveGossipKey("shared-token")

	regA := NewRegistry()
	mA, err := StartMembership(MembershipConfig{
		NodeUUID: "nodeA", BindAddr: "127.0.0.1", GossipPort: 57411, SecretKey: key,
	}, regA)
	if err != nil {
		t.Fatalf("start A: %v", err)
	}
	defer mA.Shutdown()

	// A advertises a service before B joins.
	regA.Put(sampleEntry("app.ns.svc.ns", "nodeA", "127.0.0.1", "10.30.1.5"))

	regB := NewRegistry()
	mB, err := StartMembership(MembershipConfig{
		NodeUUID: "nodeB", BindAddr: "127.0.0.1", GossipPort: 57412, SecretKey: key,
		Seeds: []string{"127.0.0.1:57411"},
	}, regB)
	if err != nil {
		t.Fatalf("start B: %v", err)
	}
	defer mB.Shutdown()

	// Join triggers an immediate push/pull state sync; poll for convergence.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(regB.TableEntriesForServiceIP("10.30.1.5")) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	entries := regB.TableEntriesForServiceIP("10.30.1.5")
	if len(entries) == 0 {
		t.Fatal("node B did not learn node A's service via gossip")
	}
	if mB.NumMembers() < 2 {
		t.Errorf("B sees %d members; want >=2", mB.NumMembers())
	}
}

// TestWrongTokenCannotJoin verifies the PSK gates admission: a node with a
// different key cannot join the mesh.
func TestWrongTokenCannotJoin(t *testing.T) {
	regA := NewRegistry()
	mA, err := StartMembership(MembershipConfig{
		NodeUUID: "nA", BindAddr: "127.0.0.1", GossipPort: 57421, SecretKey: DeriveGossipKey("right"),
	}, regA)
	if err != nil {
		t.Fatalf("start A: %v", err)
	}
	defer mA.Shutdown()

	regB := NewRegistry()
	mB, err := StartMembership(MembershipConfig{
		NodeUUID: "nB", BindAddr: "127.0.0.1", GossipPort: 57422, SecretKey: DeriveGossipKey("wrong"),
		Seeds: []string{"127.0.0.1:57421"},
	}, regB)
	if err == nil {
		defer mB.Shutdown()
	}
	// The wrong key must prevent B from forming a 2-node cluster with A.
	time.Sleep(500 * time.Millisecond)
	if mA.NumMembers() > 1 {
		t.Errorf("A admitted a wrong-key node: %d members", mA.NumMembers())
	}
}

// TestRegistryEntryLoadsLocalInstance guards the p2p source-side translation.
//
// The proxy converts a packet's SOURCE namespace IP into that instance's ServiceIP via a
// local-only lookup. In p2p mode the advertise path, which reloads the job from the
// registry, is the only thing that puts the node's own service into its own
// translation table; if this conversion loses the
// namespace IP or the InstanceNumber ServiceIP, no container can originate overlay
// traffic and every outgoing packet is silently dropped.
func TestRegistryEntryLoadsLocalInstance(t *testing.T) {
	entry := RegistryEntry{
		JobName:  "app.appns.svc.svcns",
		NodeUUID: "node-a",
		Instances: []InstanceEntry{{
			InstanceNumber: 0,
			Status:         "RUNNING",
			NamespaceIP:    "10.18.0.4",
			HostIP:         "10.99.0.11",
			HostPort:       50103,
			ServiceIP: []ServiceIPEntry{
				{Type: "RR", Address: "10.30.30.10"},
				{Type: "InstanceNumber", Address: "10.30.63.2"},
			},
		}},
	}

	reg := NewRegistry()
	reg.Put(entry)
	entries := reg.TableEntriesForJob(entry.JobName)
	if len(entries) != 1 {
		t.Fatalf("expected 1 table entry, got %d", len(entries))
	}
	got := entries[0]

	if got.Nsip.String() != "10.18.0.4" {
		t.Errorf("namespace IP not carried over: got %v", got.Nsip)
	}
	if got.Appname != "app" || got.Servicename != "svc" {
		t.Errorf("job name not split into components: %+v", got)
	}

	var instanceIP string
	for _, sip := range got.ServiceIP {
		if sip.IpType == TableEntryCache.InstanceNumber {
			instanceIP = sip.Address.String()
		}
	}
	if instanceIP != "10.30.63.2" {
		t.Errorf("InstanceNumber ServiceIP missing; source translation would fail. got %q", instanceIP)
	}
}

// TestWithdrawPropagatesAsATombstone covers undeploy in a gossip registry.
//
// Anti-entropy merges by last-writer-wins on Version and has no representation for
// absence: if a withdrawal simply deleted the key, the next exchange with a peer that
// still held the entry would put it straight back, and the service would stay advertised
// network-wide after being undeployed.
func TestWithdrawPropagatesAsATombstone(t *testing.T) {
	const job, node = "app.ns.svc.ns", "node-a"

	local := NewRegistry()
	local.Put(sampleEntry(job, node, "10.0.0.1", "10.30.30.10"))

	// A peer that has the entry and has not heard about the withdrawal yet.
	peer := NewRegistry()
	peer.Merge(local.Snapshot())

	NewResolver(local, node, nil).Withdraw(job, 0)

	if entries := local.TableEntriesForServiceIP("10.30.30.10"); len(entries) != 0 {
		t.Fatalf("withdrawn service still resolves locally: %+v", entries)
	}

	// The peer gossips its stale view back; the withdrawal must survive it.
	local.Merge(peer.Snapshot())
	if entries := local.TableEntriesForServiceIP("10.30.30.10"); len(entries) != 0 {
		t.Errorf("a peer's stale copy resurrected the withdrawn service: %+v", entries)
	}

	// And the peer must learn of the removal from us.
	peer.Merge(local.Snapshot())
	if entries := peer.TableEntriesForServiceIP("10.30.30.10"); len(entries) != 0 {
		t.Errorf("the withdrawal did not propagate to the peer: %+v", entries)
	}

	// The tombstone is not kept forever.
	tomb := peer.FindByJob(job)
	if len(tomb) != 1 || tomb[0].TTLDeadline == 0 {
		t.Fatalf("expected a TTL-bearing tombstone, got %+v", tomb)
	}
	if removed := peer.PruneExpired(tomb[0].TTLDeadline + 1); len(removed) != 1 {
		t.Errorf("tombstone was not pruned after its TTL: removed %d", len(removed))
	}
	if len(peer.FindByJob(job)) != 0 {
		t.Error("tombstone survived pruning")
	}
}
