package stats

func CalculateCpuPercentage(systemMetrics *SystemMetrics, cgroupMetrics *CgroupMetrics) float64 {
	if systemMetrics.CpuMicrosDelta == 0 || systemMetrics.OnlineCpuCount == 0 {
		return 0
	}

	systemCpuMicrosDeltaPerCore := float64(systemMetrics.CpuMicrosDelta) / float64(systemMetrics.OnlineCpuCount)
	return (float64(cgroupMetrics.CpuMicrosDelta) / systemCpuMicrosDeltaPerCore) * 100.0
}

func CalculateMemoryPercentage(systemMetrics *SystemMetrics, cgroupMetrics *CgroupMetrics) float64 {
	if systemMetrics.TotalMemoryBytes == 0 {
		return 0
	}

	return (float64(cgroupMetrics.CurrentMemoryBytes) / float64(systemMetrics.TotalMemoryBytes)) * 100.0
}
