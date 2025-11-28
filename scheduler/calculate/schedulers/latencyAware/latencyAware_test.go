package latencyAware

import (
	"fmt"
	"os"
	"scheduler/calculate/schedulers/latencyAware/bestLatencyFitLatencyAware"
	"scheduler/calculate/schedulers/latencyAware/bestMemoryFitLatencyAware"
	"scheduler/calculate/schedulers/latencyAware/lowestCarbonFitLatencyAware"
	"scheduler/calculate/schedulers/latencyAware/randomFitLatencyAware"
	"scheduler/calculate/schedulers/latencyAware/worstLatencyFitLatencyAware"
	"scheduler/calculate/schedulers/latencyAware/worstMemoryFitLatencyAware"
	"testing"
	"time"
)

func convertResourcesWorstMemoryFit(generatedResources []LatencyAwareResources) []worstMemoryFitLatencyAware.LatencyAwareResources {
	out := make([]worstMemoryFitLatencyAware.LatencyAwareResources, len(generatedResources))
	for i, s := range generatedResources {
		out[i] = worstMemoryFitLatencyAware.LatencyAwareResources{
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

func convertResourcesBestMemoryFit(generatedResources []LatencyAwareResources) []bestMemoryFitLatencyAware.LatencyAwareResources {
	out := make([]bestMemoryFitLatencyAware.LatencyAwareResources, len(generatedResources))
	for i, s := range generatedResources {
		out[i] = bestMemoryFitLatencyAware.LatencyAwareResources{
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

func convertResourcesLowestCarbonFit(generatedResources []LatencyAwareResources) []lowestCarbonFitLatencyAware.LatencyAwareResources {
	out := make([]lowestCarbonFitLatencyAware.LatencyAwareResources, len(generatedResources))
	for i, s := range generatedResources {
		out[i] = lowestCarbonFitLatencyAware.LatencyAwareResources{
			Id:              s.Id,
			JobName:         s.JobName,
			Virtualization:  s.Virtualization,
			AvailableMem:    s.AvailableMem,
			AvailableCPU:    s.AvailableCPU,
			Latency:         s.Latency,
			CarbonIntensity: s.CarbonIntensity,
		}
	}
	return out
}

func convertResourcesWorstLatencyFit(generatedResources []LatencyAwareResources) []worstLatencyFitLatencyAware.LatencyAwareResources {
	out := make([]worstLatencyFitLatencyAware.LatencyAwareResources, len(generatedResources))
	for i, s := range generatedResources {
		out[i] = worstLatencyFitLatencyAware.LatencyAwareResources{
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

func convertResourcesBestLatencyFit(generatedResources []LatencyAwareResources) []bestLatencyFitLatencyAware.LatencyAwareResources {
	out := make([]bestLatencyFitLatencyAware.LatencyAwareResources, len(generatedResources))
	for i, s := range generatedResources {
		out[i] = bestLatencyFitLatencyAware.LatencyAwareResources{
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

func convertResourcesRandomFit(generatedResources []LatencyAwareResources) []randomFitLatencyAware.LatencyAwareResources {
	out := make([]randomFitLatencyAware.LatencyAwareResources, len(generatedResources))
	for i, s := range generatedResources {
		out[i] = randomFitLatencyAware.LatencyAwareResources{
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

type result struct {
	A, B                          string
	AvgTotalMs                    float64
	AvgAms, AvgBms                float64
	AvgLatency, AvgCO2, AvgErrors float64
	DistributionIndex             float64
}

// 1️⃣ Scheduler Adapter Interface
type Scheduler interface {
	Name() string
	ConvertResources([]LatencyAwareResources) interface{}
	Calculate(job LatencyAwareResources, candidates interface{}) (string, error)
}

//
// 2️⃣ Concrete Adapters
//

// --- WorstMemoryFit ---
type WorstMemoryAdapter struct {
	Algo worstMemoryFitLatencyAware.WorstMemoryFitLatencyAware
}

func (a WorstMemoryAdapter) Name() string { return "WorstMemoryFit" }
func (a WorstMemoryAdapter) ConvertResources(res []LatencyAwareResources) interface{} {
	return convertResourcesWorstMemoryFit(res)
}
func (a WorstMemoryAdapter) Calculate(job LatencyAwareResources, candidates interface{}) (string, error) {
	jobConv := convertResourcesWorstMemoryFit([]LatencyAwareResources{job})[0]
	nodes := candidates.([]worstMemoryFitLatencyAware.LatencyAwareResources)
	res, err := a.Algo.Calculate(jobConv, nodes)
	if err != nil {
		return "", err
	}
	return res.Id, nil
}

// --- BestMemoryFit ---
type BestMemoryAdapter struct {
	Algo bestMemoryFitLatencyAware.BestMemoryFitLatencyAware
}

func (a BestMemoryAdapter) Name() string { return "BestMemoryFit" }
func (a BestMemoryAdapter) ConvertResources(res []LatencyAwareResources) interface{} {
	return convertResourcesBestMemoryFit(res)
}
func (a BestMemoryAdapter) Calculate(job LatencyAwareResources, candidates interface{}) (string, error) {
	jobConv := convertResourcesBestMemoryFit([]LatencyAwareResources{job})[0]
	nodes := candidates.([]bestMemoryFitLatencyAware.LatencyAwareResources)
	res, err := a.Algo.Calculate(jobConv, nodes)
	if err != nil {
		return "", err
	}
	return res.Id, nil
}

// --- WorstLatencyFit ---
type WorstLatencyAdapter struct {
	Algo worstLatencyFitLatencyAware.WorstLatencyFitLatencyAware
}

func (a WorstLatencyAdapter) Name() string { return "WorstLatencyFit" }
func (a WorstLatencyAdapter) ConvertResources(res []LatencyAwareResources) interface{} {
	return convertResourcesWorstLatencyFit(res)
}
func (a WorstLatencyAdapter) Calculate(job LatencyAwareResources, candidates interface{}) (string, error) {
	jobConv := convertResourcesWorstLatencyFit([]LatencyAwareResources{job})[0]
	nodes := candidates.([]worstLatencyFitLatencyAware.LatencyAwareResources)
	res, err := a.Algo.Calculate(jobConv, nodes)
	if err != nil {
		return "", err
	}
	return res.Id, nil
}

// --- BestLatencyFit ---
type BestLatencyAdapter struct {
	Algo bestLatencyFitLatencyAware.BestLatencyFitLatencyAware
}

func (a BestLatencyAdapter) Name() string { return "BestLatencyFit" }
func (a BestLatencyAdapter) ConvertResources(res []LatencyAwareResources) interface{} {
	return convertResourcesBestLatencyFit(res)
}
func (a BestLatencyAdapter) Calculate(job LatencyAwareResources, candidates interface{}) (string, error) {
	jobConv := convertResourcesBestLatencyFit([]LatencyAwareResources{job})[0]
	nodes := candidates.([]bestLatencyFitLatencyAware.LatencyAwareResources)
	res, err := a.Algo.Calculate(jobConv, nodes)
	if err != nil {
		return "", err
	}
	return res.Id, nil
}

// --- LowestCarbonFit ---
type LowestCarbonAdapter struct {
	Algo lowestCarbonFitLatencyAware.LowestCarbonFitLatencyAware
}

func (a LowestCarbonAdapter) Name() string { return "LowestCarbonFit" }
func (a LowestCarbonAdapter) ConvertResources(res []LatencyAwareResources) interface{} {
	return convertResourcesLowestCarbonFit(res)
}
func (a LowestCarbonAdapter) Calculate(job LatencyAwareResources, candidates interface{}) (string, error) {
	jobConv := convertResourcesLowestCarbonFit([]LatencyAwareResources{job})[0]
	nodes := candidates.([]lowestCarbonFitLatencyAware.LatencyAwareResources)
	res, err := a.Algo.Calculate(jobConv, nodes)
	if err != nil {
		return "", err
	}
	return res.Id, nil
}

// --- RandomFit ---
type RandomFitAdapter struct {
	Algo randomFitLatencyAware.RandomFitLatencyAware
}

func (a RandomFitAdapter) Name() string { return "RandomFit" }

func (a RandomFitAdapter) ConvertResources(res []LatencyAwareResources) interface{} {
	return convertResourcesRandomFit(res)
}

func (a RandomFitAdapter) Calculate(job LatencyAwareResources, candidates interface{}) (string, error) {
	jobConv := convertResourcesRandomFit([]LatencyAwareResources{job})[0]
	nodes := candidates.([]randomFitLatencyAware.LatencyAwareResources)
	res, err := a.Algo.Calculate(jobConv, nodes)
	if err != nil {
		return "", err
	}
	return res.Id, nil
}

// 3️⃣ Automated Two-Stage Comparison Benchmark
func BenchmarkTwoStageAllConfigs(b *testing.B) {
	algorithms := []Scheduler{
		&RandomFitAdapter{},
		&WorstMemoryAdapter{},
		&BestMemoryAdapter{},
		&WorstLatencyAdapter{},
		&BestLatencyAdapter{},
		&LowestCarbonAdapter{},
	}

	// clusters, nodesPerCluster
	configs := [][2]int{
		{1, 45},
		{3, 15},
		{5, 9},
		{9, 5},
		{15, 3},
		{45, 1},
	}

	const (
		numJobs = 30
		seed    = 0
		repeats = 10000
	)

	b.ReportAllocs()

	// ✅ Reset CSV once
	resultsFile := "benchmark_results.csv"
	header := "config,algA,algB,avg_total_ms,avg_A_ms,avg_B_ms,avg_latency,avg_co2,avg_fairness,avg_errors\n"
	_ = os.WriteFile(resultsFile, []byte(header), 0644)

	// ✅ Repeat *full sweep* of all configs N times
	for sweep := 1; sweep <= repeats; sweep++ {
		b.Logf("\n==================== FULL SWEEP %d ====================\n", sweep)

		// ✅ Now iterate through all configurations
		for _, cfg := range configs {
			clusters, nodesPerCluster := cfg[0], cfg[1]
			configLabel := fmt.Sprintf("%d-%d", clusters, nodesPerCluster)

			b.Logf("\n========== CONFIG %s (sweep %d) ==========\n", configLabel, sweep)

			// Generate fresh topology for this config + sweep
			clusterNodes, clusterSummaries := GenerateClusteredNodes(
				clusters, nodesPerCluster, seed,
			)

			// Generate jobs
			jobs := GenerateJobs(seed, numJobs)

			var totalTime float64
			var count int

			// ✅ Run all algorithm pairings
			for _, algA := range algorithms {
				for _, algB := range algorithms {

					res := runTwoStageOnce(
						b,
						algA,
						algB,
						clusterNodes,
						clusterSummaries,
						jobs,
					)

					// Log per-result
					//b.Logf("A=%-15s B=%-15s | Total=%.2f (A=%.2f B=%.2f) | Lat=%.2f | CO₂=%.2f | Fairness=%.2f | Err=%.2f",
					//	res.A, res.B,
					//	res.AvgTotalMs, res.AvgAms, res.AvgBms,
					//	res.AvgLatency, res.AvgCO2, res.DistributionIndex, res.AvgErrors,
					//)

					// ✅ Write to CSV
					appendBenchmarkCSV(resultsFile, configLabel, res)

					totalTime += res.AvgTotalMs
					count++
				}
			}

			avgTime := totalTime / float64(count)
			b.Logf("▶ CONFIG %s — Avg total scheduling time: %.2f ms\n",
				configLabel, avgTime)
		}
	}
}

// 4️⃣ Shared Runner for a Single Algorithm Pair
func runTwoStageOnce(
	b *testing.B,
	algorithmA, algorithmB Scheduler,
	baseClusterNodes [][]LatencyAwareResources,
	baseClusterSummaries []LatencyAwareResources,
	baseJobs []LatencyAwareResources,
) result {

	// Convert data for scheduling
	clusterSummariesA := algorithmA.ConvertResources(baseClusterSummaries)
	clusterNodesB := make([]interface{}, len(baseClusterNodes))
	for i := range baseClusterNodes {
		clusterNodesB[i] = algorithmB.ConvertResources(baseClusterNodes[i])
	}
	nodeCapacityMap := make(map[string]float64)
	nodeCarbonMap := make(map[string]float64)
	totalSystemCapacity := 0.0 // 🆕 Accumulator for Total System Memory

	// 1. Cluster Map
	clusterCarbonMap := make(map[string]float64)
	for _, summary := range baseClusterSummaries {
		clusterCarbonMap[summary.Id] = summary.CarbonIntensity
	}

	// 2. Node Maps
	for cIdx, cluster := range baseClusterNodes {
		parentCarbon := clusterCarbonMap[baseClusterSummaries[cIdx].Id]

		for _, node := range cluster {
			nodeCapacityMap[node.Id] = node.AvailableMem

			// 🆕 Add to total system capacity
			totalSystemCapacity += node.AvailableMem

			if node.CarbonIntensity > 0 {
				nodeCarbonMap[node.Id] = node.CarbonIntensity
			} else {
				nodeCarbonMap[node.Id] = parentCarbon
			}
		}
	}

	var (
		totalA, totalB         time.Duration
		totalErrors            int
		totalLatencySum        float64
		totalDepCount          int
		totalNormalizedCO2     float64 // Renamed for clarity
		totalJobsSuccess       int
		totalDistributionIndex float64
	)

	for i := 0; i < b.N; i++ {
		clustersCopy := deepCopyClusters(clusterNodesB)
		clusterSummaryCopy := deepCopySummaries(clusterSummariesA)

		var (
			iterTimeA, iterTimeB time.Duration
			iterationErrors      int
			iterLatencySum       float64
			iterDepCount         int
		)

		jobToNode := make(map[string]string)

		// --- Scheduling Loop ---
		for _, baseJob := range baseJobs {
			// [Stage A Logic ...]
			startA := time.Now()
			clusterID, errA := algorithmA.Calculate(baseJob, clusterSummaryCopy)
			iterTimeA += time.Since(startA)
			if errA != nil {
				iterationErrors++
				continue
			}
			deductClusterResources(clusterSummaryCopy, clusterID, baseJob.AvailableMem, baseJob.AvailableCPU)

			// [Stage B Logic ...]
			clusterIdx := getClusterIndex(clusterSummaryCopy, clusterID)
			startB := time.Now()
			nodeID, errB := algorithmB.Calculate(baseJob, clustersCopy[clusterIdx])
			iterTimeB += time.Since(startB)
			if errB != nil {
				iterationErrors++
				continue
			}

			deductNodeResources(clustersCopy[clusterIdx], nodeID, baseJob.AvailableMem, baseJob.AvailableCPU)
			jobToNode[baseJob.Id] = nodeID
			totalJobsSuccess++
		}

		// 1. Get Sum of (Used Memory * Intensity)
		iterAbsoluteCarbon := computeTotalSystemCarbon(clustersCopy, nodeCapacityMap, nodeCarbonMap)

		// 2. Normalize by Total System Capacity
		// Result unit: "Average Carbon Intensity per GB of Infrastructure"
		if totalSystemCapacity > 0 {
			totalNormalizedCO2 += (iterAbsoluteCarbon / totalSystemCapacity)
		}

		// ---------- Latency Calculation ----------
		for _, job := range baseJobs {
			srcNodeID, ok := jobToNode[job.Id]
			if !ok {
				continue
			}
			for depID := range job.Latency {
				if depID == job.Id {
					continue
				}
				depNodeID, ok := jobToNode[depID]
				if !ok {
					iterLatencySum += 6
					iterDepCount++
					continue
				}
				lat := getLatencyAdaptive(baseClusterNodes, baseClusterSummaries, srcNodeID, depNodeID)
				if lat < 0 {
					lat = 6
				}
				iterLatencySum += lat
				iterDepCount++
			}
		}

		// Accumulation
		totalA += iterTimeA
		totalB += iterTimeB
		totalErrors += iterationErrors
		totalLatencySum += iterLatencySum
		totalDepCount += iterDepCount
		totalDistributionIndex += computeDistributionIndex(baseClusterNodes, clustersCopy)
	}

	iterations := float64(b.N)

	avgLatency := 0.0
	if totalDepCount > 0 {
		avgLatency = totalLatencySum / float64(totalDepCount)
	}

	// Calculate Final Average
	avgCO2 := totalNormalizedCO2 / iterations

	avgDistributionIndex := totalDistributionIndex / iterations

	return result{
		A:                 algorithmA.Name(),
		B:                 algorithmB.Name(),
		AvgTotalMs:        float64(totalA+totalB) / iterations / 1e6,
		AvgAms:            float64(totalA) / iterations / 1e6,
		AvgBms:            float64(totalB) / iterations / 1e6,
		AvgLatency:        avgLatency,
		AvgCO2:            avgCO2,
		AvgErrors:         float64(totalErrors) / iterations,
		DistributionIndex: avgDistributionIndex,
	}
}

// 🌍 Helper: Calculates Absolute Mass (Used * Intensity)
func computeTotalSystemCarbon(
	clusters []interface{},
	capacityMap map[string]float64,
	carbonMap map[string]float64,
) float64 {
	totalScore := 0.0

	// Iterate over all clusters and nodes
	for _, cObj := range clusters {
		switch nodes := cObj.(type) {
		case []worstMemoryFitLatencyAware.LatencyAwareResources:
			for _, n := range nodes {
				totalScore += calculateNodeScore(n.Id, n.AvailableMem, capacityMap, carbonMap)
			}
		case []bestMemoryFitLatencyAware.LatencyAwareResources:
			for _, n := range nodes {
				totalScore += calculateNodeScore(n.Id, n.AvailableMem, capacityMap, carbonMap)
			}
		// ... (Include other cases: worstLatency, bestLatency, lowestCarbon, randomFit) ...
		case []worstLatencyFitLatencyAware.LatencyAwareResources:
			for _, n := range nodes {
				totalScore += calculateNodeScore(n.Id, n.AvailableMem, capacityMap, carbonMap)
			}
		case []bestLatencyFitLatencyAware.LatencyAwareResources:
			for _, n := range nodes {
				totalScore += calculateNodeScore(n.Id, n.AvailableMem, capacityMap, carbonMap)
			}
		case []lowestCarbonFitLatencyAware.LatencyAwareResources:
			for _, n := range nodes {
				totalScore += calculateNodeScore(n.Id, n.AvailableMem, capacityMap, carbonMap)
			}
		case []randomFitLatencyAware.LatencyAwareResources:
			for _, n := range nodes {
				totalScore += calculateNodeScore(n.Id, n.AvailableMem, capacityMap, carbonMap)
			}
		}
	}
	return totalScore
}

// 🌍 Helper: Calculates Used * Intensity
func calculateNodeScore(
	id string,
	currentAvail float64,
	capacityMap map[string]float64,
	carbonMap map[string]float64,
) float64 {
	totalCap, ok := capacityMap[id]
	if !ok || totalCap == 0 {
		return 0
	}

	// 1. Calculate Absolute Memory Used
	used := totalCap - currentAvail
	if used <= 0 {
		return 0
	}

	intensity := carbonMap[id]

	// 2. Return Absolute Mass (to be summed later)
	return used * intensity
}

// 5️⃣ Utility Helpers
func deepCopyClusters(src []interface{}) []interface{} {
	dst := make([]interface{}, len(src))
	for i, cluster := range src {
		switch nodes := cluster.(type) {
		case []worstMemoryFitLatencyAware.LatencyAwareResources:
			tmp := make([]worstMemoryFitLatencyAware.LatencyAwareResources, len(nodes))
			copy(tmp, nodes)
			dst[i] = tmp
		case []bestMemoryFitLatencyAware.LatencyAwareResources:
			tmp := make([]bestMemoryFitLatencyAware.LatencyAwareResources, len(nodes))
			copy(tmp, nodes)
			dst[i] = tmp
		case []worstLatencyFitLatencyAware.LatencyAwareResources:
			tmp := make([]worstLatencyFitLatencyAware.LatencyAwareResources, len(nodes))
			copy(tmp, nodes)
			dst[i] = tmp
		case []bestLatencyFitLatencyAware.LatencyAwareResources:
			tmp := make([]bestLatencyFitLatencyAware.LatencyAwareResources, len(nodes))
			copy(tmp, nodes)
			dst[i] = tmp
		case []lowestCarbonFitLatencyAware.LatencyAwareResources:
			tmp := make([]lowestCarbonFitLatencyAware.LatencyAwareResources, len(nodes))
			copy(tmp, nodes)
			dst[i] = tmp
		case []randomFitLatencyAware.LatencyAwareResources:
			tmp := make([]randomFitLatencyAware.LatencyAwareResources, len(nodes))
			copy(tmp, nodes)
			dst[i] = tmp
		default:
			dst[i] = cluster
		}
	}
	return dst
}

func deepCopySummaries(src interface{}) interface{} {
	switch s := src.(type) {
	case []worstMemoryFitLatencyAware.LatencyAwareResources:
		tmp := make([]worstMemoryFitLatencyAware.LatencyAwareResources, len(s))
		copy(tmp, s)
		return tmp
	case []bestMemoryFitLatencyAware.LatencyAwareResources:
		tmp := make([]bestMemoryFitLatencyAware.LatencyAwareResources, len(s))
		copy(tmp, s)
		return tmp
	case []worstLatencyFitLatencyAware.LatencyAwareResources:
		tmp := make([]worstLatencyFitLatencyAware.LatencyAwareResources, len(s))
		copy(tmp, s)
		return tmp
	case []bestLatencyFitLatencyAware.LatencyAwareResources:
		tmp := make([]bestLatencyFitLatencyAware.LatencyAwareResources, len(s))
		copy(tmp, s)
		return tmp
	case []lowestCarbonFitLatencyAware.LatencyAwareResources:
		tmp := make([]lowestCarbonFitLatencyAware.LatencyAwareResources, len(s))
		copy(tmp, s)
		return tmp
	case []randomFitLatencyAware.LatencyAwareResources:
		tmp := make([]randomFitLatencyAware.LatencyAwareResources, len(s))
		copy(tmp, s)
		return tmp
	default:
		return s
	}
}

func getClusterIndex(clusters interface{}, id string) int {
	switch cs := clusters.(type) {
	case []worstMemoryFitLatencyAware.LatencyAwareResources:
		for i, c := range cs {
			if c.Id == id {
				return i
			}
		}
	case []worstLatencyFitLatencyAware.LatencyAwareResources:
		for i, c := range cs {
			if c.Id == id {
				return i
			}
		}
	case []bestLatencyFitLatencyAware.LatencyAwareResources:
		for i, c := range cs {
			if c.Id == id {
				return i
			}
		}
	case []lowestCarbonFitLatencyAware.LatencyAwareResources:
		for i, c := range cs {
			if c.Id == id {
				return i
			}
		}
	case []randomFitLatencyAware.LatencyAwareResources:
		for i, c := range cs {
			if c.Id == id {
				return i
			}
		}
	case []bestMemoryFitLatencyAware.LatencyAwareResources:
		for i, c := range cs {
			if c.Id == id {
				return i
			}
		}
	}
	return -1
}

func deductClusterResources(clusters interface{}, id string, mem, cpu float64) {
	switch cs := clusters.(type) {
	case []worstMemoryFitLatencyAware.LatencyAwareResources:
		for i := range cs {
			if cs[i].Id == id {
				cs[i].AvailableMem -= mem
				cs[i].AvailableCPU -= cpu
				return
			}
		}
	case []bestMemoryFitLatencyAware.LatencyAwareResources:
		for i := range cs {
			if cs[i].Id == id {
				cs[i].AvailableMem -= mem
				cs[i].AvailableCPU -= cpu
				return
			}
		}
	case []worstLatencyFitLatencyAware.LatencyAwareResources:
		for i := range cs {
			if cs[i].Id == id {
				cs[i].AvailableMem -= mem
				cs[i].AvailableCPU -= cpu
				return
			}
		}
	case []bestLatencyFitLatencyAware.LatencyAwareResources:
		for i := range cs {
			if cs[i].Id == id {
				cs[i].AvailableMem -= mem
				cs[i].AvailableCPU -= cpu
				return
			}
		}
	case []lowestCarbonFitLatencyAware.LatencyAwareResources:
		for i := range cs {
			if cs[i].Id == id {
				cs[i].AvailableMem -= mem
				cs[i].AvailableCPU -= cpu
				return
			}
		}
	case []randomFitLatencyAware.LatencyAwareResources:
		for i := range cs {
			if cs[i].Id == id {
				cs[i].AvailableMem -= mem
				cs[i].AvailableCPU -= cpu
				return
			}
		}
	}
}

func deductNodeResources(cluster interface{}, nodeID string, mem, cpu float64) {
	switch nodes := cluster.(type) {
	case []worstMemoryFitLatencyAware.LatencyAwareResources:
		for i := range nodes {
			if nodes[i].Id == nodeID {
				nodes[i].AvailableMem -= mem
				nodes[i].AvailableCPU -= cpu
				return
			}
		}
	case []bestMemoryFitLatencyAware.LatencyAwareResources:
		for i := range nodes {
			if nodes[i].Id == nodeID {
				nodes[i].AvailableMem -= mem
				nodes[i].AvailableCPU -= cpu
				return
			}
		}
	case []worstLatencyFitLatencyAware.LatencyAwareResources:
		for i := range nodes {
			if nodes[i].Id == nodeID {
				nodes[i].AvailableMem -= mem
				nodes[i].AvailableCPU -= cpu
				return
			}
		}
	case []bestLatencyFitLatencyAware.LatencyAwareResources:
		for i := range nodes {
			if nodes[i].Id == nodeID {
				nodes[i].AvailableMem -= mem
				nodes[i].AvailableCPU -= cpu
				return
			}
		}
	case []lowestCarbonFitLatencyAware.LatencyAwareResources:
		for i := range nodes {
			if nodes[i].Id == nodeID {
				nodes[i].AvailableMem -= mem
				nodes[i].AvailableCPU -= cpu
				return
			}
		}
	case []randomFitLatencyAware.LatencyAwareResources:
		for i := range nodes {
			if nodes[i].Id == nodeID {
				nodes[i].AvailableMem -= mem
				nodes[i].AvailableCPU -= cpu
				return
			}
		}
	}
}

// baseClusterNodes: [][]LatencyAwareResources  (original nodes before scheduling)
// clustersCopy: []interface{}                  (typed slices after scheduling)
//
// Returns value in [0,1], where 1 = perfect balance.
func computeDistributionIndex(
	baseClusterNodes [][]LatencyAwareResources,
	clustersCopy []interface{},
) float64 {

	var usages []float64

	for cIdx := range clustersCopy {
		baseNodes := baseClusterNodes[cIdx]

		switch nodes := clustersCopy[cIdx].(type) {

		case []worstMemoryFitLatencyAware.LatencyAwareResources:
			for i, n := range nodes {
				used := baseNodes[i].AvailableMem - n.AvailableMem
				if used < 0 {
					used = 0
				}
				usages = append(usages, used)
			}

		case []bestMemoryFitLatencyAware.LatencyAwareResources:
			for i, n := range nodes {
				used := baseNodes[i].AvailableMem - n.AvailableMem
				if used < 0 {
					used = 0
				}
				usages = append(usages, used)
			}

		case []worstLatencyFitLatencyAware.LatencyAwareResources:
			for i, n := range nodes {
				used := baseNodes[i].AvailableMem - n.AvailableMem
				if used < 0 {
					used = 0
				}
				usages = append(usages, used)
			}

		case []lowestCarbonFitLatencyAware.LatencyAwareResources:
			for i, n := range nodes {
				used := baseNodes[i].AvailableMem - n.AvailableMem
				if used < 0 {
					used = 0
				}
				usages = append(usages, used)
			}

		case []randomFitLatencyAware.LatencyAwareResources:
			for i, n := range nodes {
				used := baseNodes[i].AvailableMem - n.AvailableMem
				if used < 0 {
					used = 0
				}
				usages = append(usages, used)
			}
		}
	}

	if len(usages) == 0 {
		return 0
	}

	var sum, sumSq float64
	for _, u := range usages {
		sum += u
		sumSq += u * u
	}

	n := float64(len(usages))
	if sumSq == 0 {
		return 1 // perfectly fair (all zero usage)
	}

	return (sum * sum) / (n * sumSq)
}

// Looks up total memory capacity of a node by ID
func getTotalMemByID(baseClusters [][]LatencyAwareResources, nodeID string) float64 {
	for _, cluster := range baseClusters {
		for _, node := range cluster {
			if node.Id == nodeID {
				return node.AvailableMem
			}
		}
	}
	return 0
}

// Looks up carbon intensity using node or cluster ID
func getCarbonIntensityByID(
	baseClusters [][]LatencyAwareResources,
	baseSummaries []LatencyAwareResources,
	nodeID, clusterID string,
) float64 {
	// Try node-level first
	for _, cluster := range baseClusters {
		for _, node := range cluster {
			if node.Id == nodeID && node.CarbonIntensity > 0 {
				return node.CarbonIntensity
			}
		}
	}
	// Fallback to cluster-level
	for _, c := range baseSummaries {
		if c.Id == clusterID && c.CarbonIntensity > 0 {
			return c.CarbonIntensity
		}
	}
	return 0
}

// Returns latency between two node IDs using base data
func getBaseLatencyByID(baseClusters [][]LatencyAwareResources, srcID, depID string) float64 {
	for _, cluster := range baseClusters {
		for _, node := range cluster {
			if node.Id == srcID {
				if lat, ok := node.Latency[depID]; ok {
					return float64(lat)
				}
			}
		}
	}
	return 0
}

// Returns latency between two node IDs, using node-level latency if in same cluster,
// and cluster-level latency if in different clusters.
func getLatencyAdaptive(
	baseClusters [][]LatencyAwareResources,
	baseClusterSummaries []LatencyAwareResources,
	srcID, depID string,
) float64 {

	// Parse cluster+node indexes directly from ID
	srcC, srcN := parseClusterNodeID(srcID)
	depC, depN := parseClusterNodeID(depID)

	// If parsing failed → penalty
	if srcC < 0 || depC < 0 || srcN < 0 || depN < 0 {
		return 6
	}

	// Bounds check
	if srcC >= len(baseClusters) || depC >= len(baseClusters) {
		return 6
	}
	if srcN >= len(baseClusters[srcC]) || depN >= len(baseClusters[depC]) {
		return 6
	}

	srcNode := baseClusters[srcC][srcN]

	// --- SAME CLUSTER → use node-level latency ---
	if srcC == depC {
		if lat, ok := srcNode.Latency[depID]; ok {
			return float64(lat)
		}
		return 6
	}

	// --- DIFFERENT CLUSTERS → use cluster-level latency ---
	srcSummary := baseClusterSummaries[srcC]
	depSummary := baseClusterSummaries[depC]

	if lat, ok := srcSummary.Latency[depSummary.Id]; ok {
		return float64(lat)
	}

	return 6
}

func parseClusterNodeID(id string) (int, int) {
	// Expected format: cluster<NUM>-node<NUM>
	var c, n int
	_, err := fmt.Sscanf(id, "cluster%d-node%d", &c, &n)
	if err != nil {
		return -1, -1
	}
	return c - 1, n - 1
}

// appendBenchmarkCSV appends a single result to the global results CSV.
func appendBenchmarkCSV(filename, config string, res result) {
	line := fmt.Sprintf("%s,%s,%s,%.4f,%.4f,%.4f,%.4f,%.4f,%.4f,%.4f\n",
		config, res.A, res.B,
		res.AvgTotalMs, res.AvgAms, res.AvgBms,
		res.AvgLatency, res.AvgCO2, res.DistributionIndex, res.AvgErrors,
	)
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Error writing to CSV: %v\n", err)
		return
	}
	defer f.Close()
	f.WriteString(line)
}
