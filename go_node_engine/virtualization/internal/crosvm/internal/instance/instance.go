package instance

import (
	"errors"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/util/iotools"
	"go_node_engine/util/ptr"
	"go_node_engine/virtualization/internal/crosvm/internal/cgroup"
	"go_node_engine/virtualization/internal/crosvm/internal/cloudinit"
	"go_node_engine/virtualization/internal/crosvm/internal/image"
	"go_node_engine/virtualization/internal/crosvm/internal/stats"
	"go_node_engine/virtualization/internal/crosvm/internal/tailbuf"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"sync"
	"time"
)

var errAlreadyClosed = errors.New("instance already closed")

type instanceStatus uint32

const (
	instanceStatusStopped instanceStatus = iota
	instanceStatusRunning instanceStatus = iota
	instanceStatusClosed  instanceStatus = iota
)

type instanceExitType uint32

const (
	instanceExitTypeNone    instanceExitType = iota
	instanceExitTypeSuccess instanceExitType = iota
	instanceExitTypeError   instanceExitType = iota
)

type instanceRestartMode uint32

const (
	instanceRestartModeNever         instanceRestartMode = iota
	instanceRestartModeUnlessStopped instanceRestartMode = iota
)

const (
	cloudInitFileName = "cloud-init.img"
)

// Instance manages one crosvm VM and supports starting, stopping (can be restarted again) and closing (permanently stopping).
// It also supports auto-restart if the VM exits by itself (not when the VM was previously stopped or closed).
//
// Locking:
//   - lock guards status, lastExit, startCount and receiving from exitChan (buffer=1)
//   - exitChan carries the next exit code, which stop and close to wait for VM completion and handleUnclaimedExit to detect when a VM exited by itself
type Instance struct {
	executablePath string

	id                  string
	service             model.Service
	statusChangeHandler func(service model.Service)
	runtimeDirPath      string
	stateDirPath        string
	restartMode         instanceRestartMode
	img                 *image.Image
	config              InstanceConfig
	configExt           InstanceConfigExt

	// Even though the exec module prevents concurrent writes from Cmd.Stdin and Cmd.Stdout,
	// we need a LockingTailBuffer, because we concurrently read from it during resource monitoring.
	outputBuffer *tailbuf.LockingTailBuffer
	logger       io.WriteCloser

	lock       sync.Mutex
	status     instanceStatus
	exitChan   chan instanceExitType
	lastExit   instanceExitType
	startCount uint32

	machineName          string
	cgroupMetricsTracker *stats.CgroupMetricsTracker
}

func NewInstance(
	id string,
	service model.Service,
	statusChangeHandler func(service model.Service),
	executablePath string,
	baseRuntimeDirPath string,
	baseStateDirPath string,
	imageStore *image.Store,
) (*Instance, error) {
	var restartMode instanceRestartMode
	if service.OneShot {
		restartMode = instanceRestartModeNever
	} else {
		restartMode = instanceRestartModeUnlessStopped
	}

	// TODO(axiphi): Currently this code is abusing the "vtpus" field of the service model to configure
	// 	 			 the disk size for the VM, as it is otherwise unused in the crosvm runtime.
	//               The left shift by 20 converts the unit of the field from MiB to bytes.
	var rootfsSize = int64(service.Vtpus) << 20
	// anything below 512MiB for the rootfs disk is likely an error, so we fail fast here
	if rootfsSize < 536870912 {
		return nil, fmt.Errorf("expected a disk size of atleast 512MiB for instance %q configured via 'vtpus' field", id)
	}

	// TODO(axiphi): Remove this directory if one of the operations below fails
	runtimeDirPath, err := iotools.CreateSubDir(baseRuntimeDirPath, fmt.Sprintf("instance-%s", id), 0o700)
	if err != nil {
		return nil, err
	}

	// TODO(axiphi): Remove this directory if one of the operations below fails
	stateDirPath, err := iotools.CreateSubDir(baseStateDirPath, fmt.Sprintf("instance-%s", id), 0o700)
	if err != nil {
		return nil, err
	}

	img, err := imageStore.Retrieve(service.Image, stateDirPath, rootfsSize)
	if err != nil {
		return nil, err
	}

	logger.InfoLogger().Printf("setting up network for instance %q...", id)
	netConf, err := setupNetwork(service)
	if err != nil {
		return nil, err
	}
	logger.InfoLogger().Printf("set up network for instance %q", id)

	config, err := NewInstanceConfig(&service, img, netConf, runtimeDirPath, stateDirPath)
	if err != nil {
		return nil, err
	}

	configExt := NewInstanceConfigExt(&service)

	machineName := cgroup.ConvertTaskIdToMachineName(id)

	inst := &Instance{
		executablePath: executablePath,

		id:                  id,
		service:             service,
		statusChangeHandler: statusChangeHandler,
		runtimeDirPath:      runtimeDirPath,
		stateDirPath:        stateDirPath,
		restartMode:         restartMode,
		img:                 img,
		config:              *config,
		configExt:           *configExt,

		outputBuffer: tailbuf.NewLockingTailBuffer(8192),
		logger: &lumberjack.Logger{
			Filename:   path.Join(stateDirPath, "instance.log"),
			MaxSize:    100,
			MaxBackups: 1,
		},

		lock:       sync.Mutex{},
		status:     instanceStatusStopped,
		exitChan:   make(chan instanceExitType, 1),
		lastExit:   instanceExitTypeNone,
		startCount: 0,

		machineName:          machineName,
		cgroupMetricsTracker: stats.NewCgroupStatsTracker(cgroup.GetMachineCgroupPath(machineName)),
	}

	// When the logs are exported for resource monitoring,
	// it skips until the first newline, to prevent partial lines from being returned.
	// To make sure the first line is returned correctly before the tailbuffer loops,
	// we prefill the outputBuffer with an initial newline.
	_, _ = inst.outputBuffer.Write([]byte{'\n'})

	if err := inst.createConfigFile(); err != nil {
		logger.ErrorLogger().Printf("could not create config file for instance %q: %v", inst.id, err)
		_ = inst.Close()
		return nil, err
	}

	ethernets := make(map[string]cloudinit.NetworkConfigEthernet)
	if netConf != nil {
		ethernets["main"] = cloudinit.NetworkConfigEthernet{
			Match: &cloudinit.NetworkConfigMatch{
				Macaddress: &netConf.Mac,
			},
			Dhcp4:     ptr.Ptr(false),
			Dhcp6:     ptr.Ptr(false),
			Addresses: []string{netConf.AddressIpv4Cidr},
			Gateway4:  &netConf.GatewayIpv4,
			Gateway6:  nil,
			Nameservers: &cloudinit.NetworkConfigNameservers{
				Addresses: []string{"8.8.8.8"}, // use google DNS, like container runtime does
			},
		}
	}

	err = cloudinit.CreateNoCloudFsImg(
		cloudinit.UserData{
			CloudInitModules: []string{
				"seed_random",
			},
			CloudConfigModules: []string{}, // needs to be empty slice and not nil to override option
			CloudFinalModules:  []string{}, // needs to be empty slice and not nil to override option
		},
		cloudinit.MetaData{
			InstanceId:    inst.id,
			LocalHostname: nil,
		},
		cloudinit.NetworkConfig{
			Version:   2,
			Ethernets: ethernets,
		},
		path.Join(inst.stateDirPath, cloudInitFileName),
	)
	if err != nil {
		logger.ErrorLogger().Printf("could not create cloud-init drive for instance %q: %v", inst.id, err)
		_ = inst.Close()
		return nil, err
	}

	return inst, nil
}

func (i *Instance) Start() error {
	i.lock.Lock()
	defer i.lock.Unlock()

	if i.status == instanceStatusRunning {
		logger.WarnLogger().Printf("ignoring instance start for %q, because it is already running", i.id)
		return nil
	}

	if i.status == instanceStatusClosed {
		logger.ErrorLogger().Printf("ignoring instance start for %q, because it is already closed", i.id)
		return errAlreadyClosed
	}

	crosvmExec := i.executablePath
	runArgs := i.generateRunArgs()
	if model.GetNodeInfo().Overlay {
		crosvmExec, runArgs = wrapCommandWithIpNetnsExec(i.service, crosvmExec, runArgs)
	}

	if os.Getenv("OAKESTRA_CROSVM_DEBUG") == "true" {
		crosvmExec, runArgs = wrapCommandWithScreen(i.id, crosvmExec, runArgs)
	}

	combinedArgs := slices.Concat([]string{crosvmExec}, runArgs)
	runCmd := cgroup.MachinedExecCommand(i.machineName, cgroup.MachineTypeVM, combinedArgs...)

	outputWriter := io.MultiWriter(i.outputBuffer, i.logger)
	runCmd.Stdout = outputWriter
	runCmd.Stderr = outputWriter

	logger.InfoLogger().Printf("starting instance %q with args %q", i.id, runArgs)
	if err := runCmd.Start(); err != nil {
		logger.ErrorLogger().Printf("failed to start instance %q: %v", i.id, err)
		return err
	}
	logger.InfoLogger().Printf("started instance %q", i.id)

	i.startCount++
	i.status = instanceStatusRunning

	go i.waitForExit(runCmd, i.startCount)

	return nil
}

func (i *Instance) Stop() error {
	i.lock.Lock()
	defer i.lock.Unlock()

	// already stopped or closed
	if i.status != instanceStatusRunning {
		return nil
	}

	if err := i.stopInternal(); err != nil {
		logger.WarnLogger().Printf("Stopping crosvm instance %q via command failed: %v", i.id, err)
	}

	select {
	case exit := <-i.exitChan:
		i.status = instanceStatusStopped
		i.lastExit = exit
		break
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timed out waiting for crosvm instance %q to stop", i.id)
	}

	return nil
}

func (i *Instance) Close() error {
	i.lock.Lock()
	defer i.lock.Unlock()

	if i.status == instanceStatusClosed {
		return nil
	}

	if i.status == instanceStatusRunning {
		if err := i.stopInternal(); err != nil {
			logger.WarnLogger().Printf("Stopping crosvm instance %q via command failed: %v", i.id, err)
		}
		select {
		case exit := <-i.exitChan:
			i.status = instanceStatusClosed
			i.lastExit = exit
			break
		case <-time.After(10 * time.Second):
			return fmt.Errorf("failed to stop crosvm instance %q", i.id)
		}
	} else if i.status == instanceStatusStopped {
		i.status = instanceStatusClosed
	}

	if err := teardownNetwork(i.service); err != nil {
		return err
	}

	iotools.CloseOrWarn(i.logger, "crosvm instance logger")
	iotools.RemoveAllOrWarn(i.runtimeDirPath)
	iotools.RemoveAllOrWarn(i.stateDirPath)

	return nil
}

func (i *Instance) WaitForExit(timeout time.Duration) error {
	i.lock.Lock()
	defer i.lock.Unlock()

	if i.status != instanceStatusRunning {
		return nil
	}

	if timeout < 0 {
		exit := <-i.exitChan
		i.exitChan <- exit
		return nil
	}

	select {
	case exit := <-i.exitChan:
		i.exitChan <- exit
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("crosvm instance didn't exit after %dms", timeout/time.Millisecond)
	}
}

func (i *Instance) GatherMetrics() (*stats.CgroupMetrics, error) {
	return i.cgroupMetricsTracker.GatherMetrics()
}

func (i *Instance) GatherLogs() string {
	output := strings.Builder{}
	// tailbuf.IsLineStart() prevents partial first line from being part of the logs
	// no error can occur jere because strings.Builder.Write doesn't error
	_, _ = i.outputBuffer.WriteToSkippingUntil(&output, tailbuf.IsLineStart())

	return output.String()
}

func (i *Instance) waitForExit(cmd *exec.Cmd, startNum uint32) {
	runErr := cmd.Wait()

	var exit instanceExitType
	if runErr == nil {
		logger.InfoLogger().Printf("instance %q exited successfully", i.id)
		exit = instanceExitTypeSuccess
	} else {
		var err *exec.ExitError
		if errors.As(runErr, &err) {
			msgBuilder := strings.Builder{}
			msgBuilder.WriteString(fmt.Sprintf("instance %q exited with error code %d, standard error excerpt:", i.id, err.ExitCode()))

			stderrBuilder := strings.Builder{}
			_, _ = i.outputBuffer.WriteToSkippingUntil(&stderrBuilder, tailbuf.IsValidUTF8Start)
			for line := range strings.Lines(stderrBuilder.String()) {
				trimmed := strings.TrimSpace(line)
				if len(trimmed) > 0 {
					msgBuilder.WriteString("\n>   ")
					msgBuilder.WriteString(trimmed)
				}
			}

			logger.ErrorLogger().Printf(msgBuilder.String())
		} else {
			logger.ErrorLogger().Printf("unexpected error when trying to run instance %q: %v", i.id, runErr)
		}
		exit = instanceExitTypeError
	}

	i.outputBuffer.Reset()
	// see NewInstance for why this initial newline is put into the buffer
	_, _ = i.outputBuffer.Write([]byte{'\n'})

	select {
	case i.exitChan <- exit:
		break
	default:
		logger.ErrorLogger().Printf(
			"instance %q exit could not be emitted into channel, this should never happen", i.id,
		)
		return
	}

	i.handleUnclaimedExit(startNum)
}

func (i *Instance) handleUnclaimedExit(startNum uint32) {
	i.lock.Lock()
	defer i.lock.Unlock()

	// If another start has happened between cmd.Wait returning in waitForExit and us acquiring the lock here,
	// this means that the value waitForExit emitted into exitChan has been consumed by someone else,
	// so they already updated status and lastExit.
	// NOTE: This should very rarely happen since it requires the following order of events:
	// 1) start
	// 2) stop/close
	// 3) handleUnclaimedExit waits on "i.lock.Lock()"
	// 4) start again (before handleUnclaimedExit resumes on "i.lock.Lock()")
	// 5) handleUnclaimedExit resumes on "i.lock.Lock()"
	if startNum != i.startCount {
		return
	}

	// If someone else has consumed the value waitForExit has emitted into exitChan,
	// they are responsible for updating the status and lastExit.
	if len(i.exitChan) == 0 {
		return
	}

	exit := <-i.exitChan
	i.status = instanceStatusStopped
	i.lastExit = exit

	if i.restartMode == instanceRestartModeNever {
		if exit == instanceExitTypeSuccess {
			i.notifyStatusChange(model.SERVICE_COMPLETED)
		} else {
			i.notifyStatusChange(model.SERVICE_DEAD)
		}
	} else {
		// since the service is not one-shot, it counts as dead and not COMPLETED
		i.notifyStatusChange(model.SERVICE_DEAD)
		defer i.restart()
	}
}

func (i *Instance) restart() {
	go func() {
		err := i.Start()
		if err != nil {
			logger.ErrorLogger().Printf("failed to restart instance %q: %v", i.id, err)
		}
	}()
}

func (i *Instance) notifyStatusChange(updatedStatus string) {
	updatedService := i.service
	updatedService.Status = updatedStatus
	i.statusChangeHandler(updatedService)
}

func (i *Instance) stopInternal() error {
	stopCmd := exec.Command(i.executablePath, i.generateStopArgs()...)
	return stopCmd.Run()
}

func (i *Instance) createConfigFile() error {
	return iotools.StoreJSONWithIndent(&i.config, path.Join(i.stateDirPath, configFileName), 0o600, "  ")
}

func (i *Instance) generateRunArgs() []string {
	return slices.Concat(
		[]string{
			"run",
			"--cfg",
			path.Join(i.stateDirPath, configFileName),
		},
		i.configExt.ToArgs(),
	)
}
func (i *Instance) generateStopArgs() []string {
	return []string{
		"stop",
		path.Join(i.runtimeDirPath, socketFileName),
	}
}

func wrapCommandWithScreen(name string, executable string, args []string) (string, []string) {
	return "screen", slices.Concat([]string{"-D", "-m", "-S", name, executable}, args)
}
