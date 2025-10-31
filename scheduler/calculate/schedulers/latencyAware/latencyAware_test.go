package latencyAware

import (
	"encoding/csv"
	"fmt"
	"os"
	"scheduler/calculate/schedulers/latencyAware/bestMemoryFitLatencyAware"
	"scheduler/calculate/schedulers/latencyAware/lowestCarbonFitLatencyAware"
	"scheduler/calculate/schedulers/latencyAware/randomFitLatencyAware"
	"scheduler/calculate/schedulers/latencyAware/worstLatencyFitLatencyAware"
	"strconv"
	"testing"
	"time"
)

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
	// --- Algorithm adapters ---
	algorithms := []Scheduler{
		&RandomFitAdapter{},
		&BestMemoryAdapter{},
		&WorstLatencyAdapter{},
		&LowestCarbonAdapter{},
	}

	// --- Cluster-node configurations (clusters-nodesPerCluster) ---
	configs := [][2]int{
		{1, 45},
		{3, 15},
		{5, 9},
		{9, 5},
		{15, 3},
		{45, 1},
	}

	const (
		numJobs = 20
		seed    = 0
	)

	b.ReportAllocs()

	// ✅ Generate jobs once and reuse across all configurations
	baseJobs := GenerateJobs(seed, numJobs)
	writeJobsCSV(baseJobs, 0, 0) // optional: export reference jobs.csv

	// ✅ Initialize CSV header for overall benchmark results
	resultsFile := "benchmark_results.csv"
	if _, err := os.Stat(resultsFile); os.IsNotExist(err) {
		header := "config,algA,algB,avg_total_ms,avg_A_ms,avg_B_ms,avg_latency,avg_co2,avg_errors\n"
		os.WriteFile(resultsFile, []byte(header), 0644)
	}

	// --- Iterate through configurations ---
	for _, cfg := range configs {
		clusters, nodesPerCluster := cfg[0], cfg[1]
		configLabel := fmt.Sprintf("%d-%d", clusters, nodesPerCluster)

		// Generate a new cluster/node topology for each config
		baseClusterNodes, baseClusterSummaries := GenerateClusteredNodes(clusters, nodesPerCluster, seed)

		// Export generated data for reproducibility
		writeClusterCSV(baseClusterSummaries, clusters, nodesPerCluster)
		writeNodesCSV(baseClusterNodes, clusters, nodesPerCluster)

		b.Logf("\n===================== CONFIG %s (clusters-nodes) =====================", configLabel)

		// --- Aggregation variables ---
		var (
			totalTime float64
			pairCount int
		)

		// --- Run all algorithm pairings ---
		for _, algA := range algorithms {
			for _, algB := range algorithms {
				res := runTwoStageOnce(
					b,
					algA,
					algB,
					baseClusterNodes,
					baseClusterSummaries,
					baseJobs, // same jobs for all configs
				)

				// Log to test output
				b.Logf("A=%-15s B=%-15s | Total=%7.2fms (A=%6.2f, B=%6.2f) | Lat=%6.2f | CO₂=%8.2f | Err=%5.2f",
					res.A, res.B,
					res.AvgTotalMs, res.AvgAms, res.AvgBms,
					res.AvgLatency, res.AvgCO2, res.AvgErrors)

				// Append to CSV for pandas/seaborn
				appendBenchmarkCSV(resultsFile, configLabel, res)

				totalTime += res.AvgTotalMs
				pairCount++
			}
		}

		// --- Summary for this configuration ---
		avgTime := 0.0
		if pairCount > 0 {
			avgTime = totalTime / float64(pairCount)
		}

		b.Logf("▶ Avg Scheduling Time across all algorithm pairings: %.2f ms\n", avgTime)
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

	// Convert data for scheduling only
	clusterSummariesA := algorithmA.ConvertResources(baseClusterSummaries)
	clusterNodesB := make([]interface{}, len(baseClusterNodes))
	for i := range baseClusterNodes {
		clusterNodesB[i] = algorithmB.ConvertResources(baseClusterNodes[i])
	}

	var (
		totalA, totalB   time.Duration
		totalErrors      int
		totalLatencySum  float64
		totalDepCount    int
		totalCO2Sum      float64
		totalJobsSuccess int
	)

	for i := 0; i < b.N; i++ {
		clustersCopy := deepCopyClusters(clusterNodesB)
		clusterSummaryCopy := deepCopySummaries(clusterSummariesA)

		var (
			iterTimeA, iterTimeB time.Duration
			iterationErrors      int
			iterLatencySum       float64
			iterDepCount         int
			iterCO2Sum           float64
		)

		jobToNode := make(map[string]string)
		jobToCluster := make(map[string]string)

		// --- Stage A + B scheduling ---
		for _, baseJob := range baseJobs {
			// ---------- Stage A ----------
			startA := time.Now()
			clusterID, errA := algorithmA.Calculate(baseJob, clusterSummaryCopy)
			iterTimeA += time.Since(startA)
			if errA != nil {
				iterationErrors++
				continue
			}

			deductClusterResources(clusterSummaryCopy, clusterID, baseJob.AvailableMem, baseJob.AvailableCPU)

			// ---------- Stage B ----------
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
			jobToCluster[baseJob.Id] = clusterID
			totalJobsSuccess++

			// ---------- CO₂ score ----------
			usedMem := baseJob.AvailableMem
			totalMem := getTotalMemByID(baseClusterNodes, nodeID)
			if totalMem == 0 {
				totalMem = usedMem // fallback
			}

			carbonIntensity := getCarbonIntensityByID(baseClusterNodes, baseClusterSummaries, nodeID, clusterID)
			if carbonIntensity > 0 {
				ratio := usedMem / totalMem
				iterCO2Sum += ratio * carbonIntensity
			}
		}

		// ---------- Latency (with penalty for unmet dependencies) ----------
		for _, job := range baseJobs {
			srcNodeID, ok := jobToNode[job.Id]
			if !ok {
				continue // job not scheduled
			}

			for depID := range job.Latency {
				if depID == job.Id {
					continue
				}

				depNodeID, ok := jobToNode[depID]
				if !ok {
					// dependency not scheduled → assign max latency penalty
					iterLatencySum += 6
					iterDepCount++
					continue
				}

				lat := getBaseLatencyByID(baseClusterNodes, srcNodeID, depNodeID)
				if lat > 0 {
					iterLatencySum += lat
				} else {
					// no latency data between these nodes → assign max penalty
					iterLatencySum += 6
				}
				iterDepCount++
			}
		}

		// Account for both double counting since latencies are symmetric
		totalLatencySum /= 2
		totalDepCount /= 2

		// --- Aggregate iteration stats ---
		totalA += iterTimeA
		totalB += iterTimeB
		totalErrors += iterationErrors
		totalLatencySum += iterLatencySum
		totalDepCount += iterDepCount
		totalCO2Sum += iterCO2Sum
	}

	// --- Final averages ---
	iterations := float64(b.N)
	avgLatency := 0.0
	if totalDepCount > 0 {
		avgLatency = totalLatencySum / float64(totalDepCount)
	}
	avgCO2 := 0.0
	if totalJobsSuccess > 0 {
		avgCO2 = totalCO2Sum / float64(totalJobsSuccess)
	}

	return result{
		A:          algorithmA.Name(),
		B:          algorithmB.Name(),
		AvgTotalMs: float64(totalA+totalB) / iterations / 1e6,
		AvgAms:     float64(totalA) / iterations / 1e6,
		AvgBms:     float64(totalB) / iterations / 1e6,
		AvgLatency: avgLatency,
		AvgCO2:     avgCO2,
		AvgErrors:  float64(totalErrors) / iterations,
	}
}

// 5️⃣ Utility Helpers
func deepCopyClusters(src []interface{}) []interface{} {
	dst := make([]interface{}, len(src))
	for i, cluster := range src {
		switch nodes := cluster.(type) {
		case []bestMemoryFitLatencyAware.LatencyAwareResources:
			tmp := make([]bestMemoryFitLatencyAware.LatencyAwareResources, len(nodes))
			copy(tmp, nodes)
			dst[i] = tmp
		case []worstLatencyFitLatencyAware.LatencyAwareResources:
			tmp := make([]worstLatencyFitLatencyAware.LatencyAwareResources, len(nodes))
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
	case []bestMemoryFitLatencyAware.LatencyAwareResources:
		tmp := make([]bestMemoryFitLatencyAware.LatencyAwareResources, len(s))
		copy(tmp, s)
		return tmp
	case []worstLatencyFitLatencyAware.LatencyAwareResources:
		tmp := make([]worstLatencyFitLatencyAware.LatencyAwareResources, len(s))
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
	case []bestMemoryFitLatencyAware.LatencyAwareResources:
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
	}
	return -1
}

func deductClusterResources(clusters interface{}, id string, mem, cpu float64) {
	switch cs := clusters.(type) {
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

func writeClusterCSV(clusters []LatencyAwareResources, c, n int) {
	fileName := fmt.Sprintf("clusters_%d-%d.csv", c, n)
	file, err := os.Create(fileName)
	if err != nil {
		fmt.Printf("Error creating %s: %v\n", fileName, err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"ClusterID", "AvailableMem", "AvailableCPU", "CarbonIntensity"})

	for _, cl := range clusters {
		writer.Write([]string{
			cl.Id,
			fmt.Sprintf("%.2f", cl.AvailableMem),
			fmt.Sprintf("%.2f", cl.AvailableCPU),
			fmt.Sprintf("%.2f", cl.CarbonIntensity),
		})
	}
}

func writeNodesCSV(clusteredNodes [][]LatencyAwareResources, c, n int) {
	fileName := fmt.Sprintf("nodes_%d-%d.csv", c, n)
	file, err := os.Create(fileName)
	if err != nil {
		fmt.Printf("Error creating %s: %v\n", fileName, err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"Cluster", "NodeID", "Mem", "CPU", "CarbonIntensity"})

	for ci, cluster := range clusteredNodes {
		for _, node := range cluster {
			writer.Write([]string{
				fmt.Sprintf("cluster%d", ci+1),
				node.Id,
				strconv.FormatFloat(node.AvailableMem, 'f', 2, 64),
				strconv.FormatFloat(node.AvailableCPU, 'f', 2, 64),
				strconv.FormatFloat(node.CarbonIntensity, 'f', 2, 64),
			})
		}
	}
}

func writeJobsCSV(jobs []LatencyAwareResources, c, n int) {
	fileName := fmt.Sprintf("jobs_%d-%d.csv", c, n)
	file, err := os.Create(fileName)
	if err != nil {
		fmt.Printf("Error creating %s: %v\n", fileName, err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"JobID", "JobName", "MemReq", "CPUReq", "NumDeps"})

	for _, job := range jobs {
		writer.Write([]string{
			job.Id,
			job.JobName,
			fmt.Sprintf("%.2f", job.AvailableMem),
			fmt.Sprintf("%.2f", job.AvailableCPU),
			strconv.Itoa(len(job.Latency)), // dependency count
		})
	}
}

// appendBenchmarkCSV appends a single result to the global results CSV.
func appendBenchmarkCSV(filename, config string, res result) {
	line := fmt.Sprintf("%s,%s,%s,%.4f,%.4f,%.4f,%.4f,%.4f,%.4f\n",
		config, res.A, res.B,
		res.AvgTotalMs, res.AvgAms, res.AvgBms,
		res.AvgLatency, res.AvgCO2, res.AvgErrors,
	)
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Error writing to CSV: %v\n", err)
		return
	}
	defer f.Close()
	f.WriteString(line)
}
