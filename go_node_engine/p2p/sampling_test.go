package p2p

import (
	"encoding/json"
	"errors"
	"go_node_engine/internal/store"
	"go_node_engine/model"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func capsEntry(t *testing.T, node string, caps Capabilities) RegistryEntry {
	t.Helper()
	data, err := json.Marshal(caps)
	if err != nil {
		t.Fatal(err)
	}
	return RegistryEntry{JobName: CapabilitiesJobName, NodeUUID: node, Meta: string(data)}
}

// fakeNetManager serves membership and a registry holding the given entries.
func fakeNetManager(t *testing.T, members []MemberInfo, entries []RegistryEntry) *NMClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/p2p/members":
			writeJSON(w, members)
		case "/p2p/services":
			writeJSON(w, entries)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return &NMClient{http: http.DefaultClient, base: srv.URL}
}

func TestCapabilitiesMatch(t *testing.T) {
	caps := Capabilities{Runtimes: []string{"docker"}, CSIDrivers: []string{"nfs"}, Vcpus: 4, MemoryMB: 2048}
	if ok, reason := caps.Match(BidRequest{Runtime: "container", Memory: 1024, Vcpus: 2, CSIDrivers: []string{"nfs"}}); !ok {
		t.Fatalf("matching job refused: %s", reason)
	}
	ok, reason := caps.Match(BidRequest{Runtime: "unikernel", Memory: 4096, Vgpus: 1, CSIDrivers: []string{"ceph"}})
	if ok {
		t.Fatal("job beyond the node's capabilities accepted")
	}
	for _, want := range []string{"runtime \"unikernel\"", "missing CSI driver \"ceph\"", "4096 MB", "vgpus"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason %q lacks %q", reason, want)
		}
	}
}

// The shortlist rules out members by gossiped capabilities and keeps members whose
// capabilities have not arrived yet.
func TestShortlistUsesGossipedCapabilities(t *testing.T) {
	setTestNode(t) // self: docker only
	uni := Capabilities{Runtimes: []string{"unikernel"}, Vcpus: 4, MemoryMB: 4096}
	members := []MemberInfo{{UUID: "self"}, {UUID: "uni"}, {UUID: "docker-only"}, {UUID: "unknown"}}
	c := &Controller{NodeUUID: "self", nm: fakeNetManager(t, members, []RegistryEntry{
		capsEntry(t, "uni", uni),
		capsEntry(t, "docker-only", Capabilities{Runtimes: []string{"docker"}, Vcpus: 4, MemoryMB: 4096}),
	})}

	candidates, ruledOut := c.shortlist(BidRequest{Runtime: "unikernel", Memory: 100, Vcpus: 1}, members)
	var got []string
	for _, m := range candidates {
		got = append(got, m.UUID)
	}
	if !slices.Equal(got, []string{"uni", "unknown"}) {
		t.Errorf("candidates = %v; want [uni unknown]", got)
	}
	if len(ruledOut) != 2 || !strings.HasPrefix(ruledOut[0], "self: runtime") || !strings.HasPrefix(ruledOut[1], "docker-only: runtime") {
		t.Errorf("ruledOut = %v", ruledOut)
	}
}

// samplingNetwork builds a coordinator ("self", docker only) and five unikernel peers
// that all answer on one fake control API, counting bids and assigns.
func samplingNetwork(t *testing.T, bidOk bool) (c *Controller, bids, assigns *atomic.Int32) {
	t.Helper()
	setTestNode(t)
	bids, assigns = &atomic.Int32{}, &atomic.Int32{}
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ctrl/bid":
			bids.Add(1)
			if bidOk {
				writeJSON(w, BidResponse{Ok: true, FreeMem: 1000, Vcpus: 2})
			} else {
				writeJSON(w, BidResponse{Reason: "needs 10 MB memory, 0 MB free"})
			}
		case "/ctrl/assign":
			assigns.Add(1)
			writeJSON(w, AssignResponse{Ok: true})
		}
	}))
	t.Cleanup(peer.Close)
	port, _ := strconv.Atoi(peer.URL[strings.LastIndex(peer.URL, ":")+1:])

	members := []MemberInfo{{UUID: "self"}}
	var entries []RegistryEntry
	for i := 1; i <= 5; i++ {
		uuid := "peer" + strconv.Itoa(i)
		members = append(members, MemberInfo{UUID: uuid, Addr: "127.0.0.1:7946"})
		entries = append(entries, capsEntry(t, uuid, Capabilities{Runtimes: []string{"unikernel"}, Vcpus: 4, MemoryMB: 4096}))
	}
	c = &Controller{
		NodeUUID:    "self",
		ControlPort: port,
		scheme:      "http",
		peers:       http.DefaultClient,
		nm:          fakeNetManager(t, members, entries),
	}
	return c, bids, assigns
}

func unikernelJob() DeployableService {
	return DeployableService{JobName: "a.b.c.d", Service: model.Service{Runtime: "unikernel", Memory: 10, Vcpus: 1}}
}

// With room in the sample, only the sampled members are asked.
func TestScheduleJobAsksOnlyTheSample(t *testing.T) {
	c, bids, assigns := samplingNetwork(t, true)
	host, err := c.ScheduleJob(unikernelJob(), false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(host, "peer") {
		t.Errorf("host = %q; want a unikernel peer", host)
	}
	if bids.Load() != defaultSchedSample || assigns.Load() != 1 {
		t.Errorf("bids=%d assigns=%d; want %d bids and 1 assign", bids.Load(), assigns.Load(), defaultSchedSample)
	}
}

// Without room in the sample, every other shortlisted member is asked before the job is
// declared unschedulable; members ruled out by capabilities are never asked.
func TestScheduleJobEscalatesBeforeGivingUp(t *testing.T) {
	c, bids, assigns := samplingNetwork(t, false)
	_, err := c.ScheduleJob(unikernelJob(), false, 0)
	if !errors.Is(err, ErrUnschedulable) {
		t.Fatalf("got %v; want ErrUnschedulable", err)
	}
	if bids.Load() != 5 || assigns.Load() != 0 {
		t.Errorf("bids=%d assigns=%d; want all 5 peers asked and no assign", bids.Load(), assigns.Load())
	}
	for _, want := range []string{"self: runtime \"unikernel\" not supported", "peer1: needs 10 MB memory"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
}

func TestPlacementWins(t *testing.T) {
	if !placementWins(3, "zzz", 2, "aaa") {
		t.Error("higher generation must win regardless of node UUID")
	}
	if !placementWins(2, "aaa", 2, "bbb") || placementWins(2, "bbb", 2, "aaa") {
		t.Error("equal generations must go to the lower node UUID")
	}
}

func TestLivePlacementSkipsWithdrawalsAndIgnoredNode(t *testing.T) {
	inst := []InstanceEntry{{Status: "RUNNING"}}
	entries := []RegistryEntry{
		{JobName: "a.b.c.d", NodeUUID: "self", Generation: 9, Instances: inst},
		{JobName: "a.b.c.d", NodeUUID: "withdrawn", Generation: 8},
		{JobName: "a.b.c.d", NodeUUID: "b", Generation: 4, Instances: inst},
		{JobName: "a.b.c.d", NodeUUID: "a", Generation: 4, Instances: inst},
		{JobName: "other", NodeUUID: "c", Generation: 7, Instances: inst},
	}
	host, gen, ok := livePlacement(entries, "a.b.c.d", "self")
	if !ok || host != "a" || gen != 4 {
		t.Errorf("livePlacement = %s, %d, %v; want a, 4, true", host, gen, ok)
	}
	if _, _, ok := livePlacement(entries, "missing", "self"); ok {
		t.Error("found a placement for a job with no entries")
	}
}

// The losing copy of a duplicated job stops itself and repoints its custody at the
// survivor; the winning copy keeps running.
func TestResolveDuplicatesStopsOnlyTheLoser(t *testing.T) {
	setTestNode(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ipam, err := NewIPAM("self", "")
	if err != nil {
		t.Fatal(err)
	}
	inst := []InstanceEntry{{Status: "RUNNING"}}
	for _, rec := range []store.Record{
		{JobName: "lose.b.c.d", Service: model.Service{Sname: "lose.b.c.d"}, Generation: 1, DesiredState: store.DesiredRunning},
		{JobName: "keep.b.c.d", Service: model.Service{Sname: "keep.b.c.d"}, Generation: 2, DesiredState: store.DesiredRunning},
	} {
		if err := store.Upsert(rec); err != nil {
			t.Fatal(err)
		}
	}
	_ = SaveCustody(CustodyRecord{Job: DeployableService{JobName: "lose.b.c.d"}, HostUUID: "self", Generation: 1})

	var stopped []string
	c := &Controller{
		NodeUUID: "self",
		ipam:     ipam,
		UndeployFn: func(sname string, _ int) error {
			stopped = append(stopped, sname)
			return nil
		},
		nm: fakeNetManager(t, nil, []RegistryEntry{
			{JobName: "lose.b.c.d", NodeUUID: "self", Generation: 1, Instances: inst},
			{JobName: "lose.b.c.d", NodeUUID: "other", Generation: 2, Instances: inst},
			{JobName: "keep.b.c.d", NodeUUID: "self", Generation: 2, Instances: inst},
			{JobName: "keep.b.c.d", NodeUUID: "other", Generation: 1, Instances: inst},
		}),
	}
	c.resolveDuplicates()

	if !slices.Equal(stopped, []string{"lose.b.c.d"}) {
		t.Fatalf("stopped %v; want only the losing copy", stopped)
	}
	records, _ := store.Load()
	if len(records) != 1 || records[0].JobName != "keep.b.c.d" {
		t.Errorf("store after resolution: %+v", records)
	}
	for _, cust := range LoadCustody() {
		if cust.Job.JobName == "lose.b.c.d" && (cust.HostUUID != "other" || cust.Generation != 2) {
			t.Errorf("custody not repointed at the survivor: %v", cust)
		}
	}
}
