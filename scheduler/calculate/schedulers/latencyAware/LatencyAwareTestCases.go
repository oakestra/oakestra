package latencyAware

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"time"
)

type LatencyAwareResources struct {
	Id              string
	JobName         string
	AvailableMem    int
	AvailableCPU    int
	Virtualization  []string
	Latency         map[string]int
	CarbonIntensity float64
}

// GenerateNodes creates a list of LatencyAwareResources (nodes).
// The total number of nodes is specified by `count`. Node sizes are proportional:
//
//	~10% large, ~60% medium, ~30% small (rounded as needed).
//
// A seed can be provided for reproducibility; if seed == 0, it uses time-based randomness.
func GenerateNodes(seed int64, count int) []LatencyAwareResources {
	if count < 3 {
		count = 3 // at least a few nodes for latency diversity
	}

	var rng *rand.Rand
	if seed == 0 {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	} else {
		rng = rand.New(rand.NewSource(seed))
	}

	// Proportional size distribution
	largeCount := int(math.Max(1, math.Round(float64(count)*0.1)))  // ~10%
	mediumCount := int(math.Max(1, math.Round(float64(count)*0.6))) // ~60%
	smallCount := count - largeCount - mediumCount
	if smallCount < 0 {
		smallCount = 0
	}

	carbonSamples := []float64{
		543.85725163, 542.49647452, 502.58034596, 459.94266318,
		813.74471178, 562.4545388, 419.57294225, 693.08914136,
		496.68364515, 430.9127515, 708.51128194, 786.98276195,
		471.7360648,
	}

	nodes := make([]LatencyAwareResources, count)

	// --- Create nodes with proportional sizes ---
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("node%d", i+1)

		var mem, cpu int
		switch {
		case i < largeCount:
			mem, cpu = 32000, 16
		case i < largeCount+mediumCount:
			mem, cpu = 8000, 4
		default:
			mem, cpu = 4000, 2
		}

		nodes[i] = LatencyAwareResources{
			Id:              id,
			AvailableMem:    mem,
			AvailableCPU:    cpu,
			Virtualization:  []string{"docker"},
			CarbonIntensity: carbonSamples[i%len(carbonSamples)],
			Latency:         make(map[string]int),
		}
	}

	// --- Generate clusters for latency grouping ---
	numClusters := int(math.Max(2, math.Round(float64(count)/9))) // ~9 nodes per cluster
	clusterIndices := make([][]int, numClusters)

	// Distribute nodes roughly evenly across clusters
	for i := 0; i < count; i++ {
		cluster := i % numClusters
		clusterIndices[cluster] = append(clusterIndices[cluster], i)
	}

	// --- Latency generation ---
	randomLatency := func() int {
		p := rng.Float64()
		switch {
		case p < 0.10:
			return 1
		case p < 0.60:
			return 2 + rng.Intn(2) // 2 or 3
		case p < 0.9:
			return 4
		default:
			return 5 + rng.Intn(2) // 5–6
		}
	}

	// --- Fill symmetric latency matrix ---
	for i := 0; i < count; i++ {
		for j := i; j < count; j++ {
			var lat int
			if i == j {
				lat = 0
			} else {
				lat = randomLatency()

				inSameCluster := false
				for _, cluster := range clusterIndices {
					foundI, foundJ := false, false
					for _, x := range cluster {
						if x == i {
							foundI = true
						}
						if x == j {
							foundJ = true
						}
					}
					if foundI && foundJ {
						inSameCluster = true
						break
					}
				}

				if !inSameCluster {
					// bump inter-cluster latency slightly
					lat += 3 + rng.Intn(3)
					if lat > 6 {
						lat = 6
					}
				}
			}

			nodes[i].Latency[nodes[j].Id] = lat
			nodes[j].Latency[nodes[i].Id] = lat
		}
	}

	return nodes
}

// GenerateJobs creates synthetic jobs with realistic dependencies and resource demands.
func GenerateJobs(seed int64, n int) []LatencyAwareResources {
	var rng *rand.Rand
	if seed == 0 {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	} else {
		rng = rand.New(rand.NewSource(seed))
	}

	jobs := make([]LatencyAwareResources, n)

	// --- Basic job creation ---
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("job%d", i+1)

		// Random resource demands (skewed towards lighter jobs)
		mem := 500 + rng.Intn(7500) // 500–8000 MB
		cpu := 1 + rng.Intn(8)      // 1–8 cores

		jobs[i] = LatencyAwareResources{
			Id:             id,
			JobName:        id,
			AvailableMem:   mem,
			AvailableCPU:   cpu,
			Virtualization: []string{"docker"},
			Latency:        make(map[string]int),
		}
	}

	// --- Random dependencies (bounded) ---
	for i := 0; i < n; i++ {
		dependencyCount := 0

		// ~35% of jobs have no dependencies
		if rng.Float64() < 0.35 {
			continue
		}

		maxDeps := 1 + rng.Intn(5) // at most 5 dependencies
		retries := 0
		for dependencyCount < maxDeps && retries < n*2 {
			retries++
			target := rng.Intn(n)
			if target == i {
				continue
			}
			if _, exists := jobs[i].Latency[jobs[target].Id]; exists {
				continue // already linked
			}

			// Random latency distribution
			lat := func() int {
				p := rng.Float64()
				switch {
				case p < 0.1:
					return 1
				case p < 0.7:
					return 2 + rng.Intn(2) // 2 or 3
				case p < 0.9:
					return 4
				default:
					return 5 + rng.Intn(2) // 5–6
				}
			}()

			// Bidirectional latency link
			jobs[i].Latency[jobs[target].Id] = lat
			jobs[target].Latency[jobs[i].Id] = lat

			dependencyCount++
		}

		if retries >= n*2 {
			// fmt.Printf("Warning: job%d hit dependency retry limit (created %d/%d deps)\n", i+1, dependencyCount, maxDeps)
		}
	}

	// --- Optional rare chained dependencies (bounded) ---
	for i := 0; i < n; i++ {
		if rng.Float64() < 0.05 { // 5% of jobs form small chains
			chainLength := 2 + rng.Intn(4) // length 2–5
			prev := i
			retries := 0

			for c := 0; c < chainLength && prev < n && retries < n*2; c++ {
				retries++
				next := rng.Intn(n)
				if next == prev {
					continue
				}
				if _, exists := jobs[prev].Latency[jobs[next].Id]; exists {
					continue
				}
				lat := 1 + rng.Intn(3)
				jobs[prev].Latency[jobs[next].Id] = lat
				jobs[next].Latency[jobs[prev].Id] = lat
				prev = next
			}
		}
	}

	return jobs
}

func ExportGraphviz(nodes []LatencyAwareResources, filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintln(f, "graph G {")

	for _, n := range nodes {
		color := "blue"
		switch {
		case n.AvailableCPU >= 16:
			color = "red"
		case n.AvailableCPU >= 4:
			color = "orange"
		default:
			color = "green"
		}
		fmt.Fprintf(f, "  %s [style=filled fillcolor=\"%s\"];\n", n.Id, color)
	}

	for _, ni := range nodes {
		for _, nj := range nodes {
			if ni.Id < nj.Id {
				fmt.Fprintf(f, "  %s -- %s [label=\"%d\"];\n", ni.Id, nj.Id, ni.Latency[nj.Id])
			}
		}
	}

	fmt.Fprintln(f, "}")
	return nil
}
