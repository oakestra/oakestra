package stats

import (
	"github.com/prometheus/procfs"
	"github.com/prometheus/procfs/sysfs"
	"os/exec"
	"strconv"
	"strings"
)

type SystemMetricsTracker struct {
	cpuTicksPerSecond uint64
	lastTotalCpuTicks uint64
}

type SystemMetrics struct {
	CpuTicksPerSecond  uint64
	CpuTicksDelta      uint64
	OnlineCpuCoreCount uint64
}

func NewSystemMetricsTracker() (*SystemMetricsTracker, error) {
	return &SystemMetricsTracker{
		cpuTicksPerSecond: obtainCpuTicksPerSecond(),
		lastTotalCpuTicks: 0,
	}, nil
}

func (s *SystemMetricsTracker) GatherMetrics() (*SystemMetrics, error) {
	newTotalCpuTicks, err := obtainTotalCpuTicks()
	if err != nil {
		return nil, err
	}

	var cpuTicksDelta uint64 = 0
	if newTotalCpuTicks > s.lastTotalCpuTicks {
		cpuTicksDelta = newTotalCpuTicks - s.lastTotalCpuTicks
	}

	onlineCpuCount, err := obtainOnlineCpuCount()
	if err != nil {
		return nil, err
	}

	s.lastTotalCpuTicks = newTotalCpuTicks
	return &SystemMetrics{
		CpuTicksPerSecond:  s.cpuTicksPerSecond,
		CpuTicksDelta:      cpuTicksDelta,
		OnlineCpuCoreCount: onlineCpuCount,
	}, nil
}

// obtainCpuTicksPerSecond obtains the ticks (jiffies) the kernel does per second.
// The correct way to do this is by calling "sysconf(_SC_CLK_TCK)" with libc,
// but this would mean that node engine needs to be compiled CGO which is a lot of setup effort.
// As an alternative, this function uses the "getconf" command with "CLK_TCK" which per Debian's documentation
// is obsolete, but still seems to work.
// If the getconf method fails, it falls back to returning 100, which is the value pretty much all Linux systems
// use anyway.
func obtainCpuTicksPerSecond() uint64 {
	getconfCpuTicksPerSecond, err := obtainCpuTicksPerSecondWithGetconf()
	if err == nil {
		return getconfCpuTicksPerSecond
	}

	return 100
}

func obtainCpuTicksPerSecondWithGetconf() (uint64, error) {
	var out strings.Builder
	getconf := exec.Command("getconf", "CLK_TCK")
	getconf.Stdout = &out

	if err := getconf.Run(); err != nil {
		return 0, err
	}

	return strconv.ParseUint(out.String(), 10, 64)
}

func obtainTotalCpuTicks() (uint64, error) {
	fs, err := procfs.NewDefaultFS()
	if err != nil {
		return 0, err
	}

	stat, err := fs.Stat()
	if err != nil {
		return 0, err
	}

	var totalCpuTicks uint64 = uint64(stat.CPUTotal.User) +
		uint64(stat.CPUTotal.Nice) +
		uint64(stat.CPUTotal.System) +
		uint64(stat.CPUTotal.IRQ) +
		uint64(stat.CPUTotal.SoftIRQ) +
		uint64(stat.CPUTotal.Idle) +
		uint64(stat.CPUTotal.Iowait) +
		uint64(stat.CPUTotal.Steal)

	return totalCpuTicks, nil
}

func obtainOnlineCpuCount() (uint64, error) {
	fs, err := sysfs.NewDefaultFS()
	if err != nil {
		return 0, err
	}

	var onlineCpuCount uint64 = 0

	cpus, err := fs.CPUs()
	for _, cpu := range cpus {
		online, err := cpu.Online()
		if err != nil || online {
			// if reading the online status fails, we also consider the cpu online
			onlineCpuCount++
		}
	}

	return onlineCpuCount, nil
}
