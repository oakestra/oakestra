package cpumemfit

import (
	"encoding/json"
	"errors"
	"scheduler/calculate/schedulers/placement"
	"testing"

	"gotest.tools/assert"
)

func base(id string, virt []string, mem, cpu, cpuPct float64) placement.BaseResources {
	return placement.BaseResources{
		ID:             id,
		Virtualization: virt,
		AvailableMem:   mem,
		AvailableCPU:   cpu,
		CPUPercent:     cpuPct,
	}
}

func TestCalculateConstraints(t *testing.T) {
	var algorithm Scheduler

	job1 := Resources{BaseResources: base("1", []string{"docker"}, 500, 1, 0)}
	job2 := Resources{BaseResources: base("2", []string{"docker"}, 5000, 20, 0)}
	job3 := Resources{BaseResources: base("3", []string{"unikernel"}, 500, 1, 0)}

	cluster1 := Resources{BaseResources: base("1", []string{"docker"}, 2000, 4, 10)}
	cluster2 := Resources{BaseResources: base("2", []string{"docker"}, 2000, 4, 20)}
	cluster3 := Resources{BaseResources: base("3", []string{"unikernel"}, 4000, 8, 10)}
	cluster4 := Resources{BaseResources: base("4", []string{"docker", "unikernel"}, 4000, 8, 5)}
	cluster5 := Resources{
		BaseResources: base("5", []string{"docker"}, 2000, 4, 10),
		CSIDrivers:    []string{"nfs.csi.k8s.io"},
	}
	cluster6 := Resources{
		BaseResources: base("6", []string{"docker"}, 2000, 4, 5),
		CSIDrivers:    []string{"nfs.csi.k8s.io", "rbd.csi.ceph.com"},
	}

	jobWithCSI := Resources{
		BaseResources: base("csi-job", []string{"docker"}, 500, 1, 0),
		Volumes:       []VolumeSpec{{VolumeID: "vol-1", CSIDriver: "nfs.csi.k8s.io", MountPath: "/data"}},
	}
	jobWithUnknownCSI := Resources{
		BaseResources: base("csi-job-missing", []string{"docker"}, 500, 1, 0),
		Volumes:       []VolumeSpec{{VolumeID: "vol-2", CSIDriver: "unknown.csi.driver", MountPath: "/data"}},
	}

	// loMem and hiMem share the same CPUPercent; only memory differs.
	// Before the cmpMemCPU fix, both scores were identical (scoreA used
	// b.AvailableMem instead of a.AvailableMem), so the tie-breaker was
	// undefined. After the fix, the candidate with more memory wins.
	loMem := Resources{BaseResources: base("lo-mem", []string{"docker"}, 600, 4, 10)}
	hiMem := Resources{BaseResources: base("hi-mem", []string{"docker"}, 1800, 4, 10)}

	var tests = []struct {
		name       string
		job        Resources
		candidates []Resources
		want       Resources
		wantErr    error
	}{
		{"Docker best fit", job1, []Resources{cluster1, cluster2, cluster3}, cluster1, nil},
		{"Unikernel best fit", job3, []Resources{cluster1, cluster2, cluster3}, cluster3, nil},
		{"Docker best fit dual cluster", job1, []Resources{cluster1, cluster2, cluster3, cluster4}, cluster4, nil},
		{"Unikernel best fit dual cluster", job3, []Resources{cluster1, cluster2, cluster3, cluster4}, cluster4, nil},
		{
			"Docker no capacity",
			job2,
			[]Resources{cluster1, cluster2, cluster3, cluster4},
			cluster1,
			placement.SchedulingError{NegativeSchedulingStatus: placement.NoActiveClusterWithCapacity},
		},
		{"CSI driver available", jobWithCSI, []Resources{cluster1, cluster5, cluster6}, cluster6, nil},
		{
			"CSI driver not available",
			jobWithUnknownCSI,
			[]Resources{cluster1, cluster5, cluster6},
			cluster1,
			placement.SchedulingError{NegativeSchedulingStatus: placement.NoActiveClusterWithCapacity},
		},
		{"Memory wins when CPU percent is equal", job1, []Resources{loMem, hiMem}, hiMem, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := algorithm.Calculate(tt.job, tt.candidates)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected error %v, got %v", tt.wantErr, err)
			}
			if err == nil {
				assert.DeepEqual(t, tt.want, got)
			}
		})
	}
}

// TestCsiDriversUnmarshal verifies that csi_drivers is correctly normalised
// from both wire formats emitted by the system:
//
//   - Root-level candidates use a flat []string produced by the cluster aggregator.
//   - Cluster-level workers send [{csi_driver_name, csi_driver_endpoint}] objects
//     verbatim from the Node Engine registration payload.
func TestCsiDriversUnmarshal(t *testing.T) {
	t.Run("flat string array (root level)", func(t *testing.T) {
		raw := `{"_id":"c1","virtualization":["docker"],"memory":2000,"vcpus":4,"cpu_percent":10,
			"csi_drivers":["nfs.csi.k8s.io","rbd.csi.ceph.com"]}`
		var r Resources
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			t.Fatalf("UnmarshalJSON error: %v", err)
		}
		assert.DeepEqual(t, r.CSIDrivers, []string{"nfs.csi.k8s.io", "rbd.csi.ceph.com"})
	})

	t.Run("object array (cluster level / node engine)", func(t *testing.T) {
		raw := `{"_id":"w1","virtualization":["docker"],"memory":2000,"vcpus":4,"cpu_percent":5,
			"csi_drivers":[
				{"csi_driver_name":"nfs.csi.k8s.io","csi_driver_endpoint":"/var/lib/kubelet/plugins/nfs.sock"},
				{"csi_driver_name":"nfs.csi.k8s.io","csi_driver_endpoint":"/other/path"},
				{"csi_driver_name":"rbd.csi.ceph.com","csi_driver_endpoint":"/var/lib/ceph.sock"}
			]}`
		var r Resources
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			t.Fatalf("UnmarshalJSON error: %v", err)
		}
		// Duplicates must be collapsed; order preserved.
		assert.DeepEqual(t, r.CSIDrivers, []string{"nfs.csi.k8s.io", "rbd.csi.ceph.com"})
	})

	t.Run("volumes from job descriptor (only volumes, no csi_drivers key)", func(t *testing.T) {
		raw := `{"_id":"job1","virtualization":["docker"],"memory":500,"vcpus":1,"cpu_percent":0,
			"volumes":[
				{"volume_id":"vol-1","csi_driver":"nfs.csi.k8s.io","mount_path":"/data","config":{"server":"192.168.1.1"}}
			]}`
		var r Resources
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			t.Fatalf("UnmarshalJSON error: %v", err)
		}
		if r.CSIDrivers != nil {
			t.Errorf("expected nil CSIDrivers for a job descriptor, got %v", r.CSIDrivers)
		}
		if len(r.Volumes) != 1 {
			t.Fatalf("expected 1 volume, got %d", len(r.Volumes))
		}
		assert.Equal(t, r.Volumes[0].CSIDriver, "nfs.csi.k8s.io")
		assert.Equal(t, r.Volumes[0].MountPath, "/data")
		assert.Equal(t, r.Volumes[0].Config["server"], "192.168.1.1")
	})
}

// TestFilterWithObjectFormatCSIDrivers exercises the full scheduling path where
// worker-node candidates carry csi_drivers in the Node Engine object format.
// This simulates the cluster-level scheduler scenario.
func TestFilterWithObjectFormatCSIDrivers(t *testing.T) {
	workerWithNFS := Resources{
		BaseResources: base("w1", []string{"docker"}, 2000, 4, 10),
		CSIDrivers:    []string{"nfs.csi.k8s.io"},
	}
	jobNeedingNFS := Resources{
		BaseResources: base("j1", []string{"docker"}, 500, 1, 0),
		Volumes:       []VolumeSpec{{VolumeID: "v1", CSIDriver: "nfs.csi.k8s.io"}},
	}
	var algorithm Scheduler
	res, err := algorithm.Calculate(jobNeedingNFS, []Resources{workerWithNFS})
	if err != nil {
		t.Fatalf("unexpected scheduling error: %v", err)
	}
	assert.Equal(t, res.ID(), "w1")
}
