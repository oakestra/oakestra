package stats

import (
	"fmt"

	"github.com/prometheus/procfs"
	"github.com/prometheus/procfs/sysfs"
)

type SystemMetricsTracker struct {
	defaultProcfs *procfs.FS
	defaultSysfs  *sysfs.FS

	lastTotalCpuSeconds float64
}

type SystemMetrics struct {
	CpuMicrosDelta   uint64
	OnlineCpuCount   uint64
	TotalMemoryBytes uint64
}

func NewSystemMetricsTracker() (*SystemMetricsTracker, error) {
	defaultProcfs, err := procfs.NewDefaultFS()
	if err != nil {
		return nil, fmt.Errorf("failed to get default procfs: %w", err)
	}

	defaultSysfs, err := sysfs.NewDefaultFS()
	if err != nil {
		return nil, fmt.Errorf("failed to get default sysfs: %w", err)
	}

	return &SystemMetricsTracker{
		defaultProcfs: &defaultProcfs,
		defaultSysfs:  &defaultSysfs,

		lastTotalCpuSeconds: 0,
	}, nil
}

func (s *SystemMetricsTracker) GatherMetrics() (*SystemMetrics, error) {
	newTotalCpuSeconds, err := s.obtainTotalCpuSeconds()
	if err != nil {
		return nil, err
	}

	var cpuSecondsDelta float64 = 0
	if newTotalCpuSeconds > s.lastTotalCpuSeconds {
		cpuSecondsDelta = newTotalCpuSeconds - s.lastTotalCpuSeconds
	}

	onlineCpuCount, err := s.obtainOnlineCpuCount()
	if err != nil {
		return nil, err
	}

	totalMemoryBytes, err := s.obtainTotalMemoryBytes()
	if err != nil {
		return nil, err
	}

	s.lastTotalCpuSeconds = newTotalCpuSeconds
	return &SystemMetrics{
		CpuMicrosDelta:   uint64(cpuSecondsDelta * float64(1000000)),
		OnlineCpuCount:   onlineCpuCount,
		TotalMemoryBytes: totalMemoryBytes,
	}, nil
}

func (s *SystemMetricsTracker) obtainTotalCpuSeconds() (float64, error) {
	stat, err := s.defaultProcfs.Stat()
	if err != nil {
		return 0, err
	}

	// the procfs library already converts the readings from /proc/stat from ticks to seconds internally
	totalCpuSeconds := stat.CPUTotal.User +
		stat.CPUTotal.Nice +
		stat.CPUTotal.System +
		stat.CPUTotal.IRQ +
		stat.CPUTotal.SoftIRQ +
		stat.CPUTotal.Idle +
		stat.CPUTotal.Iowait +
		stat.CPUTotal.Steal

	return totalCpuSeconds, nil
}

func (s *SystemMetricsTracker) obtainOnlineCpuCount() (uint64, error) {
	cpus, err := s.defaultSysfs.CPUs()
	if err != nil {
		return 0, err
	}

	var onlineCpuCount uint64 = 0
	for _, cpu := range cpus {
		online, err := cpu.Online()
		// if reading the online status fails, we also consider the cpu online
		if err != nil || online {
			onlineCpuCount++
		}
	}

	return onlineCpuCount, nil
}

func (s *SystemMetricsTracker) obtainTotalMemoryBytes() (uint64, error) {
	meminfo, err := s.defaultProcfs.Meminfo()
	if err != nil {
		return 0, err
	}

	totalMemoryBytes := meminfo.MemTotalBytes
	if totalMemoryBytes == nil {
		return 0, fmt.Errorf("total memory bytes was not available in /proc/meminfo")
	}

	return *totalMemoryBytes, nil
}
