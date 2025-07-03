package stats_test

import (
	"fmt"
	"github.com/containers/storage/pkg/reexec"
	"go_node_engine/logger"
	"go_node_engine/virtualization/internal/crosvm/internal/cgroup"
	"go_node_engine/virtualization/internal/crosvm/internal/stats"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

func init() {
	reexec.Init()
}

func Test2(t *testing.T) {
	t.Logf(cgroup.ConvertTaskIdToMachineName("test1.test2.test3.test4Lorem ipsum dolor sit amet, consectetuer adipiscing elit. Aenean commodo ligula eget dolor. Aenean massa. Cum sociis natoque penatibus et magnis dis parturient montes, nascetur ridiculus mus. Donec quam felis, ultricies nec, pellentesque eu, pretium quis, sem. Nulla consequat massa quis enim. Donec pede justo, fringilla vel, aliquet nec, vulputate eget, arcu. In enim justo, rhoncus ut, imperdiet a, venenatis vitae, justo. Nullam dictum felis eu pede mollis pretium. Integer tincidunt. Cras dapibus. Vivamus elementum semper nisi. Aenean vulputate eleifend tellus. Aenean leo ligula, porttitor eu, consequat vitae, eleifend ac, enim. Aliquam lorem ante, dapibus in, viverra quis, feugiat a, tellus. Phasellus viverra nulla ut metus varius laoreet. Quisque rutrum. Aenean imperdiet. Etiam ultricies nisi vel augue. Curabitur ullamcorper ultricies nisi. Nam eget dui. Etiam rhoncus. Maecenas tempus"))
}

func TestRunStress(t *testing.T) {
	runCmd := cgroup.MachinedExecCommand(
		"oakestratest",
		cgroup.MachineTypeVM,
		"stress",
		"--cpu",
		"4",
		"--vm",
		"8",
		"--vm-bytes",
		"1024M",
		"--vm-keep",
		"--timeout",
		"10s",
	)
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr

	if err := runCmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		t.Logf("sending SIGKILL signal to process %d", runCmd.Process.Pid)
		err := runCmd.Process.Signal(syscall.SIGKILL)
		if err != nil {
			t.Logf("failed to send SIGKILL signal to process: %v", err)
		}
	}()

	exitChan := make(chan error, 1)
	go func() {
		exitChan <- runCmd.Wait()
	}()

	systemTracker, err := stats.NewSystemMetricsTracker()
	if err != nil {
		t.Fatal(err)
	}

	cgroupPath := cgroup.GetMachineCgroupPath("oakestratest")
	cgroupTracker, err := stats.NewCgroupStatsTracker(cgroupPath)
	if err != nil {
		t.Fatal(err)
	}

outer:
	for {
		select {
		case err := <-exitChan:
			if err != nil {
				t.Fatal(err)
			}
			break outer
		case <-time.After(1 * time.Second):
			systemMetrics, err := systemTracker.GatherMetrics()
			if err != nil {
				logger.WarnLogger().Printf("failed to gather system metrics: %v", err)
				continue
			}

			cgroupMetrics, err := cgroupTracker.GatherMetrics()
			if err != nil {
				logger.WarnLogger().Printf("failed to gather cgroup metrics: %v", err)
				continue
			}

			var infoMsg strings.Builder
			infoMsg.WriteString("collected metrics:\n")
			_, _ = fmt.Fprintf(&infoMsg, "  > System:\n")
			_, _ = fmt.Fprintf(&infoMsg, "    * CpuMicrosDelta: %d\n", systemMetrics.CpuMicrosDelta)
			_, _ = fmt.Fprintf(&infoMsg, "    * OnlineCpuCount: %d\n", systemMetrics.OnlineCpuCount)
			_, _ = fmt.Fprintf(&infoMsg, "    * TotalMemoryBytes: %d\n", systemMetrics.TotalMemoryBytes)
			_, _ = fmt.Fprintf(&infoMsg, "  > Cgroup:\n")
			_, _ = fmt.Fprintf(&infoMsg, "    * CpuMicrosDelta: %d\n", cgroupMetrics.CpuMicrosDelta)
			_, _ = fmt.Fprintf(&infoMsg, "    * CurrentMemoryBytes: %d\n", cgroupMetrics.CurrentMemoryBytes)
			_, _ = fmt.Fprintf(&infoMsg, "    * CpuPercentage: %.2f%%\n", stats.CalculateCpuPercentage(systemMetrics, cgroupMetrics))
			_, _ = fmt.Fprintf(&infoMsg, "    * MemoryPercentage: %.2f%%\n", stats.CalculateMemoryPercentage(systemMetrics, cgroupMetrics))
			_, _ = fmt.Fprintf(&infoMsg, " -\n")
			_, _ = fmt.Fprintf(&infoMsg, " -\n")
			logger.InfoLogger().Print(infoMsg.String())
		}
	}

}
