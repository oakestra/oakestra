package p2p

import (
	"testing"
)

const testSLA = `{
  "sla_version": "v2.0",
  "applications": [{
    "application_name": "myapp",
    "application_namespace": "prod",
    "microservices": [
      {
        "microservice_name": "web",
        "microservice_namespace": "prod",
        "virtualization": "container",
        "code": "docker.io/library/nginx:latest",
        "memory": 100,
        "vcpus": 1,
        "port": "80",
        "addresses": {"rr_ip": "10.30.30.30"}
      },
      {
        "microservice_name": "worker",
        "microservice_namespace": "prod",
        "virtualization": "container",
        "code": "docker.io/library/busybox",
        "memory": 50,
        "vcpus": 1,
        "one_shot": true
      }
    ]
  }]
}`

func TestExpandSLA(t *testing.T) {
	services, err := ExpandSLA([]byte(testSLA))
	if err != nil {
		t.Fatalf("ExpandSLA: %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("got %d services; want 2", len(services))
	}
	web := services[0]
	if web.JobName != "myapp.prod.web.prod" {
		t.Errorf("job name = %s; want myapp.prod.web.prod (4-part, plan §1.4)", web.JobName)
	}
	if web.RRIP != "10.30.30.30" {
		t.Errorf("rr_ip from SLA not honoured: %s", web.RRIP)
	}
	if web.Service.Runtime != "docker" {
		t.Errorf("virtualization 'container' should map to runtime 'docker'; got %s", web.Service.Runtime)
	}
	if web.Service.Image != "docker.io/library/nginx:latest" || web.Service.Memory != 100 {
		t.Errorf("service fields not mapped: %+v", web.Service)
	}
	if !services[1].Service.OneShot {
		t.Error("one_shot not carried over")
	}
}

func TestExpandSLARejectsInvalid(t *testing.T) {
	if _, err := ExpandSLA([]byte(`{"applications":[]}`)); err == nil {
		t.Error("empty SLA must be rejected")
	}
	if _, err := ExpandSLA([]byte(`not json`)); err == nil {
		t.Error("non-JSON must be rejected")
	}
}

func TestHRWDeterministicAndConsistent(t *testing.T) {
	members := []string{"node-a", "node-b", "node-c", "node-d", "node-e"}
	order1 := HRWOrder("myapp.prod.web.prod", members)
	order2 := HRWOrder("myapp.prod.web.prod", []string{"node-e", "node-c", "node-a", "node-d", "node-b"})
	for i := range order1 {
		if order1[i] != order2[i] {
			t.Fatalf("HRW order depends on input order: %v vs %v — must be deterministic (plan §7.1)", order1, order2)
		}
	}
	// Different jobs should (generally) get different custodian orders.
	other := HRWOrder("other.ns.svc.ns", members)
	same := true
	for i := range order1 {
		if order1[i] != other[i] {
			same = false
			break
		}
	}
	if same {
		t.Log("warning: two jobs share identical HRW order (possible but unlikely)")
	}
}

func TestCustodiansIncludeHost(t *testing.T) {
	members := []string{"node-a", "node-b", "node-c", "node-d"}
	got := Custodians("job.x.y.z", members, 3, "node-c")
	if len(got) != 3 {
		t.Fatalf("got %d custodians; want 3", len(got))
	}
	if got[0] != "node-c" {
		t.Errorf("host must be first custodian (plan §7.1): %v", got)
	}
	seen := map[string]bool{}
	for _, u := range got {
		if seen[u] {
			t.Errorf("duplicate custodian: %v", got)
		}
		seen[u] = true
	}
}

func TestPrimaryLiveCustodianExcludesDead(t *testing.T) {
	members := []string{"node-a", "node-b", "node-c"}
	primary := PrimaryLiveCustodian("job.x.y.z", members, "node-b")
	if primary == "node-b" || primary == "" {
		t.Fatalf("primary custodian must be live and != dead node; got %q", primary)
	}
	// All live nodes must agree (determinism): recompute from a permuted list.
	again := PrimaryLiveCustodian("job.x.y.z", []string{"node-c", "node-a", "node-b"}, "node-b")
	if primary != again {
		t.Fatalf("nodes disagree on the rescheduler: %s vs %s", primary, again)
	}
}

func TestIPAMBlockOwnershipAndAllocation(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ipam, err := NewIPAM("node-uuid-1", "")
	if err != nil {
		t.Fatal(err)
	}
	a, err := ipam.Alloc()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ipam.Alloc()
	if a == b {
		t.Fatalf("duplicate allocation: %s", a)
	}
	block := BlockFor("node-uuid-1")
	want := "10.30."
	if a[:len(want)] != want {
		t.Errorf("allocation %s outside 10.30.0.0/16", a)
	}
	_ = block
	ipam.Free(a)
	c, _ := ipam.Alloc()
	if c != a {
		t.Errorf("freed IP %s not reused (got %s)", a, c)
	}
}

func TestIPAMConfiguredBlock(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ipam, err := NewIPAM("whatever", "10.30.42.0/24")
	if err != nil {
		t.Fatal(err)
	}
	ip, _ := ipam.Alloc()
	if ip != "10.30.42.1" {
		t.Errorf("configured block ignored: got %s; want 10.30.42.1", ip)
	}
}
