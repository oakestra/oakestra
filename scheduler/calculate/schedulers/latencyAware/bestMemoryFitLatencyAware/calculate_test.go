package bestMemoryFitLatencyAware

import (
	"errors"
	"math"
	"scheduler/calculate/schedulers/interfaces"
	"scheduler/calculate/schedulers/latencyAware"
	"testing"
	"time"

	"gotest.tools/assert"
)

func convertResources(generatedResources []latencyAware.LatencyAwareResources) []LatencyAwareResources {
	out := make([]LatencyAwareResources, len(generatedResources))
	for i, s := range generatedResources {
		out[i] = LatencyAwareResources{
			Id:             s.Id,
			JobName:        s.JobName,
			Virtualization: s.Virtualization,
			AvailableMem:   s.AvailableMem,
			AvailableCPU:   s.AvailableCPU,
			Latency:        s.Latency,
		}
	}
	return out
}

func BenchmarkCalculateWorstFitLatencyAware(b *testing.B) {
	const numJobs = 20
	const numNodes = 100

	b.ReportAllocs()

	// Generate a static pool of candidate nodes
	generatedNodes := latencyAware.GenerateNodes(0, numNodes)
	nodes := convertResources(generatedNodes)

	algorithm := BestMemoryFitLatencyAware{}

	var (
		durations   []float64
		totalErrors int
	)

	b.ResetTimer() // only time what's inside the loop

	for i := 0; i < b.N; i++ {
		// Make a deep copy of nodes for this run
		candidates := make([]LatencyAwareResources, len(nodes))
		copy(candidates, nodes)

		// Generate a fresh set of jobs
		generatedJobs := latencyAware.GenerateJobs(0, numJobs)
		jobs := convertResources(generatedJobs)

		start := time.Now()
		errorsInRun := 0

		for _, job := range jobs {
			res, err := algorithm.Calculate(job, candidates)
			if err != nil {
				errorsInRun++
				continue
			}

			// Update node resources
			for i := range candidates {
				if candidates[i].Id == res.Id {
					candidates[i].AvailableMem -= job.AvailableMem
					candidates[i].AvailableCPU -= job.AvailableCPU
					break
				}
			}
		}

		duration := time.Since(start).Seconds() * 1000 // ms

		durations = append(durations, duration)
		totalErrors += errorsInRun

		//b.Logf("Iteration %d: %.3f ms total for %d jobs (%d errors). N = %d", i+1, duration, numJobs, errorsInRun, b.N)
	}

	// --- Compute statistics ---
	mean, stddev := calcStats(durations)
	avgErrors := float64(totalErrors) / float64(b.N)

	b.StopTimer()
	b.Logf("\n==== Benchmark Summary ====")
	b.Logf("Total iterations: %d", b.N)
	b.Logf("Average time: %.3f ms (stddev: %.3f ms)", mean, stddev)
	b.Logf("Average errors per run: %.2f", avgErrors)
	b.Logf("Total errors: %d", totalErrors)
	b.Logf("===========================\n")
}

// calcStats computes the mean and standard deviation for a slice of float64s.
func calcStats(values []float64) (mean, stddev float64) {
	if len(values) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean = sum / float64(len(values))

	var variance float64
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(values))
	stddev = math.Sqrt(variance)
	return mean, stddev
}

func TestCalculateWorstFitLatencyAware(t *testing.T) {
	var algorithm BestMemoryFitLatencyAware

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
		Latency:        map[string]int{"job4": 3},
	}

	job4 := LatencyAwareResources{
		Id:             "job4",
		JobName:        "job4",
		AvailableMem:   4_000,
		AvailableCPU:   2,
		Virtualization: []string{"docker"},
		Latency:        map[string]int{"job3": 3},
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

	node1 := LatencyAwareResources{
		Id:           "node1",
		AvailableMem: 32_000,
		AvailableCPU: 16,
		Latency: map[string]int{
			"node1": 0,
			"node2": 1,
			"node3": 2,
			"node4": 4,
			"node5": 5,
		},
		Virtualization: []string{"docker"},
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
		Virtualization: []string{"docker"},
	}
	node3 := LatencyAwareResources{
		Id:           "node3",
		AvailableMem: 8_000,
		AvailableCPU: 4,
		Latency: map[string]int{
			"node1": 2,
			"node2": 4,
			"node3": 0,
			"node4": 3,
			"node5": 5,
		},
		Virtualization: []string{"docker"},
	}
	node4 := LatencyAwareResources{
		Id:           "node4",
		AvailableMem: 4_000,
		AvailableCPU: 2,
		Latency: map[string]int{
			"node1": 4,
			"node2": 3,
			"node3": 3,
			"node4": 0,
			"node5": 2,
		},
		Virtualization: []string{"docker"},
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
		Virtualization: []string{"docker"},
	}

	var tests = []struct {
		name       string
		setup      []LatencyAwareResources
		job        LatencyAwareResources
		candidates []LatencyAwareResources
		res        LatencyAwareResources
		error      error
	}{
		{"Trivial worst fit", []LatencyAwareResources{}, job1, []LatencyAwareResources{node1, node2, node3, node4, node5}, node4, nil},
		{"Trivial no candidate", []LatencyAwareResources{}, job2, []LatencyAwareResources{node1, node3, node4, node5}, node1, interfaces.SchedulingError{NegativeSchedulingStatus: interfaces.NoActiveClusterWithCapacity}},
		{"Interdependant a", []LatencyAwareResources{job3}, job4, []LatencyAwareResources{node1, node2, node3, node4, node5}, node2, nil},
		{"Interdependant b", []LatencyAwareResources{job3, job4}, job4, []LatencyAwareResources{node1, node2, node3, node4, node5}, node2, nil},
		{"Spoke and wheel dependencies a", []LatencyAwareResources{job3, job4, job4, job4}, job4, []LatencyAwareResources{node1, node2, node3, node4, node5}, node3, nil},
		{"Spoke and wheel dependencies b", []LatencyAwareResources{job3, job4, job4, job4, job4, job4}, job4, []LatencyAwareResources{node1, node2, node3, node4, node5}, node5, nil},
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
