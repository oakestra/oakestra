package rootBestCpuMemFit

import (
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
