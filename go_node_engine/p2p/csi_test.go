package p2p

import (
	"go_node_engine/config"
	"go_node_engine/model"
	"go_node_engine/p2p/scheduling"
	"testing"
)

const testCSISLA = `{
  "sla_version": "v2.0",
  "applications": [{
    "application_name": "myapp",
    "application_namespace": "prod",
    "microservices": [{
      "microservice_name": "db",
      "microservice_namespace": "prod",
      "virtualization": "container",
      "code": "docker.io/library/postgres:16",
      "memory": 100,
      "vcpus": 1,
      "volumes": [{"volume_id": "data", "csi_driver": "nfs.csi.k8s.io", "mount_path": "/var/lib/postgresql/data"}]
    }]
  }]
}`

func TestExpandSLAKeepsVolumesAndBidCarriesCSIDrivers(t *testing.T) {
	services, err := ExpandSLA([]byte(testCSISLA))
	if err != nil {
		t.Fatalf("ExpandSLA: %v", err)
	}
	vols := services[0].Service.Volumes
	if len(vols) != 1 || vols[0].CSIDriver != "nfs.csi.k8s.io" || vols[0].MountPath != "/var/lib/postgresql/data" {
		t.Fatalf("volumes not carried into the service: %+v", vols)
	}
	req := newBidRequest(services[0])
	if len(req.CSIDrivers) != 1 || req.CSIDrivers[0] != "nfs.csi.k8s.io" {
		t.Fatalf("bid request CSI drivers = %v; want [nfs.csi.k8s.io]", req.CSIDrivers)
	}
}

func TestEvaluateBidRequiresCSIDriver(t *testing.T) {
	node := model.GetNodeInfo()
	node.Technology = []config.RuntimeType{model.CONTAINER_RUNTIME}
	node.MemoryMB, node.MemoryUsed, node.CpuCores = 4096, 0, 4
	node.CSIDrivers = nil
	t.Cleanup(func() { node.CSIDrivers = nil })

	c := &Controller{NodeUUID: "self"}
	req := BidRequest{Runtime: string(model.CONTAINER_RUNTIME), Memory: 100, Vcpus: 1, CSIDrivers: []string{"nfs.csi.k8s.io"}}

	if resp := c.evaluateBid(req); resp.Ok {
		t.Fatal("node without the CSI driver must not bid")
	}
	node.AddCSIDriver(config.CSIDriverType{Name: "nfs.csi.k8s.io"})
	resp := c.evaluateBid(req)
	if !resp.Ok {
		t.Fatal("node with the CSI driver should bid")
	}
	if len(resp.CSIDrivers) != 1 || resp.CSIDrivers[0] != "nfs.csi.k8s.io" {
		t.Fatalf("bid response CSI drivers = %v; want [nfs.csi.k8s.io]", resp.CSIDrivers)
	}
}

// The coordinator filters too, so a positive bid from a peer lacking the driver
// (e.g. an older binary) is never chosen.
func TestPickHostSkipsBidsWithoutCSIDriver(t *testing.T) {
	req := scheduling.Requirements{Runtime: "docker", Memory: 100, Vcpus: 1, CSIDrivers: []string{"nfs.csi.k8s.io"}}
	bids := []scheduling.Bid{
		{NodeUUID: "big-no-driver", Mem: 8000, Vcpus: 8},
		{NodeUUID: "small-with-driver", Mem: 1000, Vcpus: 2, CSIDrivers: []string{"nfs.csi.k8s.io"}},
	}
	for i := 0; i < 20; i++ {
		if got, ok := scheduling.PickHost(req, bids, 0); !ok || got != "small-with-driver" {
			t.Fatalf("PickHost = %q, %v; want small-with-driver", got, ok)
		}
	}
	if _, ok := scheduling.PickHost(req, bids[:1], 0); ok {
		t.Fatal("PickHost must fail when no bid has the CSI driver")
	}
}
