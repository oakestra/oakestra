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
	AvailableMem    float64
	AvailableCPU    float64
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

		var mem, cpu float64
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

// GenerateClusteredNodes creates a set of clusters, each containing a number of latency-aware nodes.
//
// Arguments:
//
//	clusters         number of clusters
//	nodesPerCluster  number of nodes per cluster
//	seed             random seed (0 = time-based random)
//
// Returns:
//   - [][]LatencyAwareResources: list of clusters, each containing its nodes
//   - []LatencyAwareResources: cluster-level aggregate summaries
func GenerateClusteredNodes(clusters int, nodesPerCluster int, seed int64) ([][]LatencyAwareResources, []LatencyAwareResources) {
	if clusters < 1 {
		clusters = 1
	}
	if nodesPerCluster < 1 {
		nodesPerCluster = 1
	}

	var rng *rand.Rand
	if seed == 0 {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	} else {
		rng = rand.New(rand.NewSource(seed))
	}

	carbonSamples := []float64{
		543.85725163, 502.58034596, 459.94266318,
		813.74471178, 562.4545388, 419.57294225, 693.08914136,
		496.68364515, 430.9127515, 708.51128194, 786.98276195,
		471.7360648,
	}

	// Helper for weighted latency distribution (1–6, mostly 2–3)
	randomLatency := func() int {
		p := rng.Float64()
		switch {
		case p < 0.10:
			return 1
		case p < 0.60:
			return 2 + rng.Intn(2) // 2–3
		case p < 0.9:
			return 4
		default:
			return 5 + rng.Intn(2) // 5–6
		}
	}

	// --- Build clusters ---
	clusteredNodes := make([][]LatencyAwareResources, clusters)
	nodeCounter := 0

	for c := 0; c < clusters; c++ {
		nodes := make([]LatencyAwareResources, nodesPerCluster)

		// Cluster composition biases
		biasLarge := rng.Float64() * 0.25
		biasSmall := rng.Float64() * 0.35
		biasMedium := 1.0 - biasLarge - biasSmall
		if biasMedium < 0 {
			biasMedium = 0.1
		}

		clusterCarbon := carbonSamples[c%len(carbonSamples)]

		for i := 0; i < nodesPerCluster; i++ {
			nodeCounter++
			id := fmt.Sprintf("cluster%d-node%d", c+1, i+1)

			r := rng.Float64()
			var mem, cpu float64
			switch {
			case r < biasLarge:
				mem, cpu = 32000, 16 // large
			case r < biasLarge+biasMedium:
				mem, cpu = 8000, 4 // medium
			default:
				mem, cpu = 4000, 2 // small
			}

			variation := 1.0 + (rng.Float64()*0.3 - 0.15) // range [0.85, 1.15]
			nodeCarbon := clusterCarbon * variation

			nodes[i] = LatencyAwareResources{
				Id:              id,
				AvailableMem:    mem,
				AvailableCPU:    cpu,
				Virtualization:  []string{"docker"},
				CarbonIntensity: nodeCarbon,
				Latency:         make(map[string]int),
			}
		}

		// --- Symmetric intra-cluster latency matrix ---
		for i := range nodes {
			for j := i; j < len(nodes); j++ {
				lat := 0
				if i != j {
					lat = randomLatency()
				}
				nodes[i].Latency[nodes[j].Id] = lat
				nodes[j].Latency[nodes[i].Id] = lat
			}
		}
		clusteredNodes[c] = nodes
	}

	// --- Aggregate cluster summaries ---
	clusterSummaries := make([]LatencyAwareResources, clusters)
	for c := 0; c < clusters; c++ {
		clusterID := fmt.Sprintf("cluster%d", c+1)
		var totalMem, totalCPU float64
		var totalCarbon float64

		for _, n := range clusteredNodes[c] {
			totalMem += n.AvailableMem
			totalCPU += n.AvailableCPU
			totalCarbon += n.CarbonIntensity
		}
		avgCarbon := totalCarbon / float64(nodesPerCluster)

		clusterSummaries[c] = LatencyAwareResources{
			Id:              clusterID,
			AvailableMem:    totalMem,
			AvailableCPU:    totalCPU,
			Virtualization:  []string{"docker"},
			CarbonIntensity: avgCarbon, // NEW: averaged over nodes
			Latency:         make(map[string]int),
		}
	}

	// --- Inter-cluster latency (symmetric, high bias) ---
	for i := 0; i < clusters; i++ {
		for j := i; j < clusters; j++ {
			var lat int
			if i == j {
				lat = 0
			} else {
				p := rng.Float64()
				switch {
				case p < 0.1:
					lat = 3
				case p < 0.6:
					lat = 4
				case p < 0.9:
					lat = 5
				default:
					lat = 6
				}
			}
			clusterSummaries[i].Latency[clusterSummaries[j].Id] = lat
			clusterSummaries[j].Latency[clusterSummaries[i].Id] = lat
		}
	}

	return clusteredNodes, clusterSummaries
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

	// --- Basic job creation (weighted low) ---
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("job%d", i+1)

		// Weighted random helper (skews toward small)
		weighted := func(min, max float64) float64 {
			r := rng.Float64()
			r = r * r * r // cubic weighting — heavy bias to smaller numbers
			return min + (max-min)*r
		}

		mem := weighted(500, 8000) // MB, heavily biased to smaller values
		cpu := weighted(0, 4)      // vCPU, same bias

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

		// ~40% of jobs have no dependencies
		if rng.Float64() < 0.40 {
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

			// Random latency distribution (favoring smaller values)
			lat := func() int {
				p := rng.Float64()
				switch {
				case p < 0.4:
					return 1
				case p < 0.8:
					return 2 + rng.Intn(2) // 2–3
				case p < 0.95:
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
