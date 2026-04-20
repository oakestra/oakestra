package rootBestCpuMemFit

import (
	"encoding/json"
	"errors"
	"scheduler/calculate/schedulers/interfaces"
	"testing"

	"gotest.tools/assert"
)

func TestCalculateConstraints(t *testing.T) {
	var algorithm BestCpuMemFit
	job1 := CpuMemResources{
		Constraints:    nil,
		Id:             "1",
		Virtualization: []string{"docker"},
		AvailableMem:   500,
		AvailableCPU:   1,
	}
	job2 := CpuMemResources{
		Constraints:    nil,
		Id:             "2",
		Virtualization: []string{"docker"},
		AvailableMem:   5000,
		AvailableCPU:   20,
	}
	job3 := CpuMemResources{
		Constraints:    nil,
		Id:             "3",
		Virtualization: []string{"unikernel"},
		AvailableMem:   500,
		AvailableCPU:   1,
	}
	cluster1 := CpuMemResources{
		Constraints:    nil,
		Id:             "1",
		Virtualization: []string{"docker"},
		AvailableMem:   2000,
		AvailableCPU:   4,
		CPUPercent:     10,
	}
	cluster2 := CpuMemResources{
		Constraints:    nil,
		Id:             "2",
		Virtualization: []string{"docker"},
		AvailableMem:   2000,
		AvailableCPU:   4,
		CPUPercent:     20,
	}
	cluster3 := CpuMemResources{
		Constraints:    nil,
		Id:             "3",
		Virtualization: []string{"unikernel"},
		AvailableMem:   4000,
		AvailableCPU:   8,
		CPUPercent:     10,
	}
	cluster4 := CpuMemResources{
		Constraints:    nil,
		Id:             "4",
		Virtualization: []string{"docker", "unikernel"},
		AvailableMem:   4000,
		AvailableCPU:   8,
		CPUPercent:     5,
	}
	cluster5 := CpuMemResources{
		Constraints:    nil,
		Id:             "5",
		Virtualization: []string{"docker"},
		AvailableMem:   2000,
		AvailableCPU:   4,
		CPUPercent:     10,
		CSIDrivers:     []string{"nfs.csi.k8s.io"},
	}
	cluster6 := CpuMemResources{
		Constraints:    nil,
		Id:             "6",
		Virtualization: []string{"docker"},
		AvailableMem:   2000,
		AvailableCPU:   4,
		CPUPercent:     5,
		CSIDrivers:     []string{"nfs.csi.k8s.io", "rbd.csi.ceph.com"},
	}

	jobWithCSI := CpuMemResources{
		Constraints:    nil,
		Id:             "csi-job",
		Virtualization: []string{"docker"},
		AvailableMem:   500,
		AvailableCPU:   1,
		Volumes: []VolumeSpec{
			{VolumeID: "vol-1", CSIDriver: "nfs.csi.k8s.io", MountPath: "/data"},
		},
	}
	jobWithUnknownCSI := CpuMemResources{
		Constraints:    nil,
		Id:             "csi-job-missing",
		Virtualization: []string{"docker"},
		AvailableMem:   500,
		AvailableCPU:   1,
		Volumes: []VolumeSpec{
			{VolumeID: "vol-2", CSIDriver: "unknown.csi.driver", MountPath: "/data"},
		},
	}

	var tests = []struct {
		name       string
		job        CpuMemResources
		candidates []CpuMemResources
		res        CpuMemResources
		error      error
	}{
		{"Docker best fit", job1, []CpuMemResources{cluster1, cluster2, cluster3}, cluster1, nil},
		{"Unikernel best fit", job3, []CpuMemResources{cluster1, cluster2, cluster3}, cluster3, nil},
		{"Docker best fit dual cluster", job1, []CpuMemResources{cluster1, cluster2, cluster3, cluster4}, cluster4, nil},
		{"Unikernel best fit dual cluster", job3, []CpuMemResources{cluster1, cluster2, cluster3, cluster4}, cluster4, nil},
		{"Docker no capacity", job2, []CpuMemResources{cluster1, cluster2, cluster3, cluster4}, cluster1, interfaces.SchedulingError{NegativeSchedulingStatus: interfaces.NoActiveClusterWithCapacity}},
		// CSI driver matching
		{"CSI driver available", jobWithCSI, []CpuMemResources{cluster1, cluster5, cluster6}, cluster6, nil},
		{"CSI driver not available", jobWithUnknownCSI, []CpuMemResources{cluster1, cluster5, cluster6}, cluster1, interfaces.SchedulingError{NegativeSchedulingStatus: interfaces.NoActiveClusterWithCapacity}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := algorithm.Calculate(tt.job, tt.candidates)
			if !errors.Is(err, tt.error) {
				t.Errorf("Expected error %s, but got %s", tt.error, err)
			}
			if err == nil {
				assert.DeepEqual(t, tt.res, res)
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
		var r CpuMemResources
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
		var r CpuMemResources
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
		var r CpuMemResources
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
	// Worker candidate whose csi_drivers arrived as objects and were normalised
	// to strings during UnmarshalJSON.
	workerWithNFS := CpuMemResources{
		Id:             "w1",
		Virtualization: []string{"docker"},
		AvailableMem:   2000,
		AvailableCPU:   4,
		CPUPercent:     10,
		CSIDrivers:     []string{"nfs.csi.k8s.io"},
	}
	jobNeedingNFS := CpuMemResources{
		Id:             "j1",
		Virtualization: []string{"docker"},
		AvailableMem:   500,
		AvailableCPU:   1,
		Volumes:        []VolumeSpec{{VolumeID: "v1", CSIDriver: "nfs.csi.k8s.io"}},
	}
	var algorithm BestCpuMemFit
	res, err := algorithm.Calculate(jobNeedingNFS, []CpuMemResources{workerWithNFS})
	if err != nil {
		t.Fatalf("unexpected scheduling error: %v", err)
	}
	assert.Equal(t, res.Id, "w1")
}
