package stats

import (
	"bufio"
	"fmt"
	"go_node_engine/util/iotools"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
)

type CgroupMetricsTracker struct {
	cgroupPath         string
	lastTotalCpuMicros uint64
}

type CgroupMetrics struct {
	CpuMicrosDelta uint64
}

func NewCgroupStatsTracker(cgroupPath string) (*CgroupMetricsTracker, error) {
	return &CgroupMetricsTracker{
		cgroupPath:         cgroupPath,
		lastTotalCpuMicros: 0,
	}, nil
}

func (c *CgroupMetricsTracker) GatherMetrics() (*CgroupMetrics, error) {
	newTotalCpuMicros, err := obtainTotalCpuMicros(c.cgroupPath)
	if err != nil {
		return nil, fmt.Errorf("failed to gather metrics for cgroup %q: %w", c.cgroupPath, err)
	}

	var cpuMicrosDelta uint64 = 0
	if newTotalCpuMicros > c.lastTotalCpuMicros {
		cpuMicrosDelta = newTotalCpuMicros - c.lastTotalCpuMicros
	}

	c.lastTotalCpuMicros = newTotalCpuMicros
	return &CgroupMetrics{
		CpuMicrosDelta: cpuMicrosDelta,
	}, nil

	//utime := (cpuStats.userUsec * 100) / c.kernelTicksPerSecond
	//stime := (cpuStats.systemUsec * 100) / c.kernelTicksPerSecond
	//cpuPercentage := float32(saturatingSub(utime+stime, lasttimes)) / (float32(period) * 100.0)
}

func obtainTotalCpuMicros(cgroupPath string) (uint64, error) {
	cpuStatPath := path.Join(cgroupPath, "cpu.stat")
	cpuStatFile, err := os.OpenFile(cpuStatPath, os.O_RDONLY, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to read cpu.stat file in cgroup %q: %w", cgroupPath, err)
	}
	defer iotools.CloseOrWarn(cpuStatFile, cpuStatPath)

	cpuStatScanner := bufio.NewScanner(cpuStatFile)
	for cpuStatScanner.Scan() {
		cpuStatLine := cpuStatScanner.Text()
		cpuStatParts := strings.SplitN(cpuStatLine, " ", 2)
		if len(cpuStatParts) != 2 {
			log.Printf("failed to parse cpu.stat line in cgroup %q: %s", cgroupPath, cpuStatLine)
			continue
		}

		cpuStatKey := cpuStatParts[0]
		cpuStatValue := cpuStatParts[1]

		if cpuStatKey == "usage_usec" {
			usageUsec, err := strconv.ParseUint(cpuStatValue, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("failed to parse usage_usec value in cpu.stat file of cgroup %q: %w", cgroupPath, err)
			}
			return usageUsec, nil
		}
	}

	return 0, fmt.Errorf("failed to find usage_usec line in cpu.stat file of cgroup %q", cgroupPath)
}
