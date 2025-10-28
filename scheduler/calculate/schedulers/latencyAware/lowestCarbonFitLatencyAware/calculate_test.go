package lowestCarbonFitLatencyAware

import (
	"errors"
	"scheduler/calculate/schedulers/interfaces"
	"testing"

	"gotest.tools/assert"
)

func TestCalculateLowestCarbonFitLatencyAware(t *testing.T) {
	var algorithm LowestCarbonFitLatencyAware

	// Minimal requirements, no dependencies
	job1 := LatencyAwareResources{
		Id:             "job1",
		JobName:        "job1",
		AvailableMem:   2_000,
		AvailableCPU:   1,
		Virtualization: []string{"docker"},
	}

	// Impossible requirements, no dependencies
	job2 := LatencyAwareResources{
		Id:             "job2",
		JobName:        "job2",
		AvailableMem:   2_000,
		AvailableCPU:   1,
		Virtualization: []string{"unikernel"},
	}

	job3 := LatencyAwareResources{
		Id:             "job3",
		JobName:        "job3",
		AvailableMem:   4_000,
		AvailableCPU:   2,
		Virtualization: []string{"docker"},
		Latency:        map[string]int{"job4": 2},
	}

	job4 := LatencyAwareResources{
		Id:             "job4",
		JobName:        "job4",
		AvailableMem:   4_000,
		AvailableCPU:   2,
		Virtualization: []string{"docker"},
		Latency:        map[string]int{"job3": 2},
	}

	job5 := LatencyAwareResources{
		Id:             "job5",
		JobName:        "job5",
		AvailableMem:   8_000,
		AvailableCPU:   4,
		Virtualization: []string{"docker"},
		Latency:        map[string]int{"job6": 1},
	}

	job6 := LatencyAwareResources{
		Id:             "job6",
		JobName:        "job6",
		AvailableMem:   8_000,
		AvailableCPU:   4,
		Virtualization: []string{"docker"},
		Latency:        map[string]int{"job5": 1},
	}

	job7 := LatencyAwareResources{
		Id:             "job7",
		JobName:        "job7",
		AvailableMem:   4_000,
		AvailableCPU:   2,
		Virtualization: []string{"docker"},
	}

	job8 := LatencyAwareResources{
		Id:             "job8",
		JobName:        "job8",
		AvailableMem:   4_000,
		AvailableCPU:   2,
		Virtualization: []string{"docker"},
		Latency:        map[string]int{"job7": 2},
	}

	job9 := LatencyAwareResources{
		Id:             "job9",
		JobName:        "job9",
		AvailableMem:   4_000,
		AvailableCPU:   2,
		Virtualization: []string{"docker"},
		Latency:        map[string]int{"job7": 3},
	}

	job10 := LatencyAwareResources{
		Id:             "job10",
		JobName:        "job10",
		AvailableMem:   4_000,
		AvailableCPU:   2,
		Virtualization: []string{"docker"},
		Latency: map[string]int{
			"job7": 3,
			"job8": 5,
		},
	}

	node1 := LatencyAwareResources{
		Id:           "node1",
		AvailableMem: 32_000,
		AvailableCPU: 16,
		Latency: map[string]int{
			"node1": 0,
			"node2": 1,
			"node3": 2,
			"node4": 3,
			"node5": 5,
		},
		Virtualization:  []string{"docker"},
		CarbonIntensity: 543.8572516300001, //eastus
	}
	node2 := LatencyAwareResources{
		Id:           "node2",
		AvailableMem: 8_000,
		AvailableCPU: 4,
		Latency: map[string]int{
			"node1": 1,
			"node2": 0,
			"node3": 4,
			"node4": 3,
			"node5": 5,
		},
		Virtualization:  []string{"docker"},
		CarbonIntensity: 502.58034596, //westus2
	}
	node3 := LatencyAwareResources{
		Id:           "node3",
		AvailableMem: 8_000,
		AvailableCPU: 4,
		Latency: map[string]int{
			"node1": 2,
			"node2": 4,
			"node3": 0,
			"node4": 4,
			"node5": 5,
		},
		Virtualization:  []string{"docker"},
		CarbonIntensity: 813.74471178, //australiaeast
	}
	node4 := LatencyAwareResources{
		Id:           "node4",
		AvailableMem: 4_000,
		AvailableCPU: 2,
		Latency: map[string]int{
			"node1": 3,
			"node2": 3,
			"node3": 4,
			"node4": 0,
			"node5": 2,
		},
		Virtualization:  []string{"docker"},
		CarbonIntensity: 459.94266318, //westus3
	}
	node5 := LatencyAwareResources{
		Id:           "node5",
		AvailableMem: 8_000,
		AvailableCPU: 4,
		Latency: map[string]int{
			"node1": 5,
			"node2": 5,
			"node3": 5,
			"node4": 2,
			"node5": 0,
		},
		Virtualization:  []string{"docker"},
		CarbonIntensity: 562.4545388, //northeurope
	}

	var tests = []struct {
		name       string
		setup      []LatencyAwareResources
		job        LatencyAwareResources
		candidates []LatencyAwareResources
		res        LatencyAwareResources
		error      error
	}{
		{"Trivial lowest co2 fit", []LatencyAwareResources{}, job1, []LatencyAwareResources{node1, node2, node3, node4, node5}, node4, nil},
		{"Trivial no candidate", []LatencyAwareResources{}, job2, []LatencyAwareResources{node1, node3, node4, node5}, node1, interfaces.SchedulingError{NegativeSchedulingStatus: interfaces.NoActiveClusterWithCapacity}},
		{"Interdependant a", []LatencyAwareResources{job3}, job4, []LatencyAwareResources{node1, node2, node3, node4, node5}, node5, nil},
		{"Interdependant b", []LatencyAwareResources{job3, job4}, job4, []LatencyAwareResources{node1, node2, node3, node4, node5}, node5, nil},
		//{"Spoke and wheel dependencies a", []LatencyAwareResources{job3, job4, job4, job4, job4}, job4, []LatencyAwareResources{node1, node2, node3, node4, node5}, node5, interfaces.SchedulingError{NegativeSchedulingStatus: interfaces.NoActiveClusterWithCapacity}},
		//{"Spoke and wheel dependencies b", []LatencyAwareResources{job3, job4, job4, job4, job4, job4, job4}, job4, []LatencyAwareResources{node1, node2, node3, node4, node5}, node1, interfaces.SchedulingError{NegativeSchedulingStatus: interfaces.NoActiveClusterWithCapacity}},
		{"Spoke and wheel dependencies c", []LatencyAwareResources{job7, job8, job9}, job10, []LatencyAwareResources{node1, node2, node3, node4, node5}, node2, nil},
		{"MTP Latency Requirement", []LatencyAwareResources{job5}, job6, []LatencyAwareResources{node1, node2, node3, node4, node5}, node1, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// place candidates before interested calculation
			for _, j := range tt.setup {
				res, err := algorithm.Calculate(j, tt.candidates)
				if err != nil {
					t.Fatalf("Setup failed for %s: %v", tt.name, err)
				}

				// find and update the candidate
				for i := range tt.candidates {
					if tt.candidates[i].Id == res.Id {
						tt.candidates[i].AvailableMem -= j.AvailableMem
						tt.candidates[i].AvailableCPU -= j.AvailableCPU
						break
					}
				}
			}

			res, err := algorithm.Calculate(tt.job, tt.candidates)
			if !errors.Is(err, tt.error) {
				t.Errorf("Expected error %s, but got %s", tt.error, err)
			}
			if err == nil {
				assert.DeepEqual(t, tt.res.Id, res.Id)
			}
		})
	}
}
