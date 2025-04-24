package crosvminstance

// #cgo pkg-config: /opt/oakestra/lib/pkgconfig/crosvm_control.pc
// #include <crosvm_control.h>
// #include <stdlib.h>
import "C"
import (
	"encoding/json"
	"errors"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/util/runtimedir"
	"os"
	"os/exec"
	"path"
	"slices"
	"sync"
	"time"
	"unsafe"
)

var errAlreadyClosed = errors.New("instance already closed")

const configFileName = "config.json"
const socketFileName = "instance.sock"

type instanceStatus int

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

// Instance manages one crosvm VM and supports starting, stopping (can be restarted again) and closing (permanently stopping).
// It also supports auto-restart if the VM exits by itself (not when the VM was previously stopped or closed).
//
// Locking:
//   - lock guards status, lastExit, startCount and receiving from exitChan (buffer=1)
//   - exitChan carries the next exit code, which stop and close to wait for VM completion and handleUnclaimedExit to detect when a VM exited by itself
type Instance struct {
	executablePath string

	id             string
	runtimeDirPath string
	restartMode    instanceRestartMode
	config         InstanceConfig
	configExt      InstanceConfigExt

	lock       sync.Mutex
	status     instanceStatus
	exitChan   chan instanceExitType
	lastExit   instanceExitType
	startCount uint32
}

func NewInstance(
	id string,
	service *model.Service,
	executablePath string,
	baseRuntimeDirPath string,
) (*Instance, error) {
	runtimeDirPath, err := runtimedir.CreateSubRuntimeDir(baseRuntimeDirPath, fmt.Sprintf("instance-%s", id))
	if err != nil {
		return nil, err
	}

	var restartMode instanceRestartMode
	if service.OneShot {
		restartMode = instanceRestartModeNever
	} else {
		restartMode = instanceRestartModeUnlessStopped
	}

	instance := &Instance{
		executablePath: executablePath,

		id:             id,
		runtimeDirPath: runtimeDirPath,
		restartMode:    restartMode,
		config:         InstanceConfig{},
		configExt:      InstanceConfigExt{},

		lock:       sync.Mutex{},
		status:     instanceStatusStopped,
		exitChan:   make(chan instanceExitType, 1),
		lastExit:   instanceExitTypeNone,
		startCount: 0,
	}

	if err := instance.createConfigFile(); err != nil {
		logger.ErrorLogger().Printf("rt-crosvm: Could not create config file for instance %q: %v", instance.id, err)
		_ = instance.Close()
		return nil, err
	}

	return instance, nil
}

func (i *Instance) Start() error {
	i.lock.Lock()
	defer i.lock.Unlock()

	if i.status == instanceStatusRunning {
		logger.WarnLogger().Printf("rt-crosvm: Ignoring instance start for %q, because it is already running", i.id)
		return nil
	}

	if i.status == instanceStatusClosed {
		logger.ErrorLogger().Printf("rt-crosvm: Ignoring instance start for %q, because it is already closed", i.id)
		return errAlreadyClosed
	}

	runArgs := i.generateRunArgs()
	runCmd := exec.Command(i.executablePath, runArgs...)

	logger.InfoLogger().Printf("rt-crosvm: Starting instance %q with args %q", i.id, runArgs)
	if err := runCmd.Start(); err != nil {
		logger.ErrorLogger().Printf("rt-crosvm: Failed to start instance %q: %v", i.id, err)
		return err
	}
	logger.InfoLogger().Printf("rt-crosvm: Started instance %q", i.id)

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

	if !i.callCrosvmStop() {
		logger.WarnLogger().Printf("rt-crosvm: Stopping crosvm instance %q via library failed", i.id)
	}

	select {
	case exit := <-i.exitChan:
		i.status = instanceStatusStopped
		i.lastExit = exit
		break
	case <-time.After(10 * time.Second):
		return fmt.Errorf("rt-crosvm: Failed to stop crosvm instance %q", i.id)
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
		if !i.callCrosvmStop() {
			logger.WarnLogger().Printf("rt-crosvm: Stopping crosvm instance %q via library failed", i.id)
		}
		select {
		case exit := <-i.exitChan:
			i.status = instanceStatusClosed
			i.lastExit = exit
			break
		case <-time.After(10 * time.Second):
			return fmt.Errorf("rt-crosvm: Failed to stop crosvm instance %q", i.id)
		}
	} else if i.status == instanceStatusStopped {
		i.status = instanceStatusClosed
	}

	if err := os.RemoveAll(i.runtimeDirPath); err != nil {
		logger.WarnLogger().Printf("rt-crosvm: Failed to remove runtime directory %q of instance %q: %v", i.runtimeDirPath, i.id, err)
	}
	return nil
}

func (i *Instance) waitForExit(cmd *exec.Cmd, startNum uint32) {
	runErr := cmd.Wait()

	var exit instanceExitType
	if runErr == nil {
		logger.InfoLogger().Printf("rt-crosvm: Instance %q exited successfully", i.id)
		exit = instanceExitTypeSuccess
	} else {
		var err *exec.ExitError
		if errors.As(runErr, &err) {
			logger.ErrorLogger().Printf("rt-crosvm: Instance %q exited with error: %v", i.id, runErr)
		} else {
			logger.ErrorLogger().Printf("rt-crosvm: Unexpected error when trying to run instance %q: %v", i.id, runErr)
		}
		exit = instanceExitTypeError
	}

	select {
	case i.exitChan <- exit:
		break
	default:
		logger.ErrorLogger().Printf(
			"rt-crosvm: Instance %q exit could not be emitted into channel, this should never happen", i.id,
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

	if i.restartMode == instanceRestartModeUnlessStopped {
		defer i.restart()
	}
}

func (i *Instance) restart() {
	go func() {
		err := i.Start()
		if err != nil {
			logger.ErrorLogger().Printf("rt-crosvm: Failed to restart instance %q: %v", i.id, err)
		}
	}()
}

func (i *Instance) callCrosvmStop() bool {
	socketPath := C.CString(path.Join(i.runtimeDirPath, socketFileName))
	defer C.free(unsafe.Pointer(socketPath))

	return bool(C.crosvm_client_stop_vm(socketPath))
}

func (i *Instance) createConfigFile() error {
	configPath := path.Join(i.runtimeDirPath, configFileName)
	configFile, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := configFile.Close(); err != nil {
			logger.WarnLogger().Printf("virtualization-crosvm: failed to close config file %q: %v", configPath, err)
		}
	}()

	configEncoder := json.NewEncoder(configFile)
	err = configEncoder.Encode(i.config)
	if err != nil {
		return err
	}

	return nil
}

func (i *Instance) generateRunArgs() []string {
	return slices.Concat(
		[]string{
			"run",
			"--cfg",
			path.Join(i.runtimeDirPath, configFileName),
		},
		i.configExt.toArgs(),
	)
}
