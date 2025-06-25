package stats

func CalculateCpuPercentage(systemMetrics *SystemMetrics, cgroupMetrics *CgroupMetrics) float64 {
	if systemMetrics.CpuTicksDelta == 0 || systemMetrics.OnlineCpuCoreCount == 0 {
		return 0
	}

	var systemCpuSeconds float64 = float64(systemMetrics.CpuTicksDelta) / float64(systemMetrics.CpuTicksPerSecond)
	var systemCpuMicros float64 = systemCpuSeconds * 1000000
	var systemCpuMicrosPerCore = systemCpuMicros / float64(systemMetrics.OnlineCpuCoreCount)

	return (float64(cgroupMetrics.CpuMicrosDelta) / systemCpuMicrosPerCore) * 100.0
}
