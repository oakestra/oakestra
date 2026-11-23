package p2p

import (
	"encoding/json"
	"errors"
	"go_node_engine/config"
	"go_node_engine/model"
	"go_node_engine/p2p/scheduling"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const hugeMemory = 1 << 40

// setTestNode gives the node-info singleton a known capacity for the test.
func setTestNode(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	node := model.GetNodeInfo()
	node.Technology = []config.RuntimeType{model.CONTAINER_RUNTIME}
	// Memory is re-read from the host on every GetNodeInfo call, so tests request
	// hugeMemory to force a refusal instead of pinning the node's memory.
	node.CpuCores = 2
	node.CSIDrivers = nil
	t.Cleanup(func() { node.CSIDrivers = nil })
}

func TestEvaluateBidExplainsRefusal(t *testing.T) {
	setTestNode(t)
	c := &Controller{NodeUUID: "self"}

	resp := c.evaluateBid(BidRequest{Runtime: "unikernel", Memory: hugeMemory, Vcpus: 4, Vgpus: 1000, CSIDrivers: []string{"nfs.csi.k8s.io"}})
	if resp.Ok {
		t.Fatal("bid should be refused")
	}
	for _, want := range []string{`runtime "unikernel"`, `missing CSI driver "nfs.csi.k8s.io"`, "needs 1099511627776 MB memory", "needs 4 vcpus, node has 2", "needs 1000 vgpus"} {
		if !strings.Contains(resp.Reason, want) {
			t.Errorf("reason %q lacks %q", resp.Reason, want)
		}
	}

	// "container" is the SLA alias of the docker runtime.
	for _, runtime := range []string{"docker", "container"} {
		if resp := c.evaluateBid(BidRequest{Runtime: runtime, Memory: 1, Vcpus: 1}); !resp.Ok || resp.Reason != "" {
			t.Fatalf("%s bid should be accepted without reason, got %+v", runtime, resp)
		}
	}
}

func TestBidRequestFromQuery(t *testing.T) {
	if req, err := bidRequestFromQuery(url.Values{"network": {"1"}}); err != nil || req != nil {
		t.Fatalf("no job params: got %+v, %v; want nil", req, err)
	}
	req, err := bidRequestFromQuery(url.Values{"runtime": {"container"}, "memory": {"256"}, "vgpus": {"2"}, "csi": {"a", "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if req.Runtime != "docker" || req.Memory != 256 || req.Vcpus != 0 || req.Vgpus != 2 || len(req.CSIDrivers) != 2 {
		t.Fatalf("got %+v", req)
	}
	if _, err := bidRequestFromQuery(url.Values{"memory": {"lots"}}); err == nil {
		t.Fatal("invalid memory should be rejected")
	}
}

// A network sweep answers for self locally, queries reachable peers with the same
// job, and reports unreachable ones instead of failing.
func TestCapacityHandlerNetworkSweep(t *testing.T) {
	setTestNode(t)

	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("network") || r.URL.Query().Get("memory") != strconv.Itoa(hugeMemory) {
			t.Errorf("peer got unexpected query %q", r.URL.RawQuery)
		}
		writeJSON(w, Capacity{NodeUUID: "peer", Bid: &BidResponse{Ok: true}})
	}))
	defer peer.Close()
	port, _ := strconv.Atoi(peer.URL[strings.LastIndex(peer.URL, ":")+1:])

	netManager := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []MemberInfo{
			{UUID: "self", Addr: "127.0.0.1:7946"},
			{UUID: "peer", Addr: "127.0.0.1:7946"},
			{UUID: "gone", Addr: "127.0.0.2:7946"},
		})
	}))
	defer netManager.Close()

	c := &Controller{
		NodeUUID:    "self",
		ControlPort: port,
		scheme:      "http",
		peers:       http.DefaultClient,
		nm:          &NMClient{http: http.DefaultClient, base: netManager.URL},
	}
	rec := httptest.NewRecorder()
	c.capacityHandler(rec, httptest.NewRequest(http.MethodGet, "/ctrl/capacity?network=1&memory="+strconv.Itoa(hugeMemory), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var network NetworkCapacity
	if err := json.NewDecoder(rec.Body).Decode(&network); err != nil {
		t.Fatal(err)
	}
	if network.Sample != defaultSchedSample || len(network.Members) != 3 {
		t.Fatalf("got %+v", network)
	}
	self, fromPeer, gone := network.Members[0], network.Members[1], network.Members[2]
	if self.Capacity == nil || self.Capacity.FreeMemMB <= 0 || self.Capacity.Bid == nil || self.Capacity.Bid.Ok {
		t.Errorf("self row: %+v", self.Capacity)
	}
	if fromPeer.Capacity == nil || fromPeer.Capacity.NodeUUID != "peer" || !fromPeer.Capacity.Bid.Ok {
		t.Errorf("peer row: %+v (err %q)", fromPeer.Capacity, fromPeer.Error)
	}
	if gone.Capacity != nil || gone.Error == "" {
		t.Errorf("unreachable row should carry an error: %+v", gone)
	}
}

func TestPickHostSkipsBidsWithoutEnoughGPUs(t *testing.T) {
	req := scheduling.Requirements{Runtime: "docker", Memory: 100, Vcpus: 1, Vgpus: 1}
	bids := []scheduling.Bid{
		{NodeUUID: "big-no-gpu", Mem: 8000, Vcpus: 8},
		{NodeUUID: "small-with-gpu", Mem: 1000, Vcpus: 2, Vgpus: 1},
	}
	for i := 0; i < 20; i++ {
		if got, ok := scheduling.PickHost(req, bids, 0); !ok || got != "small-with-gpu" {
			t.Fatalf("PickHost = %q, %v; want small-with-gpu", got, ok)
		}
	}
	if len(bids) != 2 {
		t.Fatal("PickHost must not modify the caller's bids")
	}
	if _, ok := scheduling.PickHost(req, bids[:1], 0); ok {
		t.Fatal("PickHost must fail when no bid has enough GPUs")
	}
}

func TestCredsAccessError(t *testing.T) {
	old := credsDirPath
	credsDirPath = t.TempDir()
	t.Cleanup(func() { credsDirPath = old })

	if err := CredsAccessError(); err != nil {
		t.Fatalf("missing creds (pre-genesis) must not be an error, got %v", err)
	}
	path := filepath.Join(credsDirPath, "creds.json")
	if err := os.WriteFile(path, []byte("{}"), 0o000); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		t.Skip("root can read any file")
	}
	if err := CredsAccessError(); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("unreadable creds: got %v; want a permission error", err)
	}
}

// A single-node round (no NetManager membership) must still say why it failed.
func TestScheduleJobUnschedulableCarriesReasons(t *testing.T) {
	setTestNode(t)
	c := &Controller{NodeUUID: "self", nm: &NMClient{http: http.DefaultClient, base: "http://127.0.0.1:1"}}
	job := DeployableService{JobName: "a.b.c.d", Service: model.Service{Runtime: "docker", Memory: hugeMemory, Vcpus: 1}}

	_, err := c.ScheduleJob(job, false, 0)
	if !errors.Is(err, ErrUnschedulable) {
		t.Fatalf("got %v; want ErrUnschedulable", err)
	}
	if !strings.Contains(err.Error(), "self: needs 1099511627776 MB memory") {
		t.Fatalf("error lacks the decline reason: %v", err)
	}
}
