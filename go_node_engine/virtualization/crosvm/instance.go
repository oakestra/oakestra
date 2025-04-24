package crosvm

// #cgo pkg-config: /opt/oakestra/lib/pkgconfig/crosvm_control.pc
// #include <crosvm_control.h>
import "C"
import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/util"
	"os"
	"os/exec"
	"path"
	"slices"
	"sync"
	"sync/atomic"
	"time"
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

type instanceExit uint32

const (
	instanceExitNone    instanceExit = iota
	instanceExitSuccess instanceExit = iota
	instanceExitError   instanceExit = iota
)

type instance struct {
	id             uuid.UUID
	taskId         string
	runtimeDirPath string
	config         InstanceConfig
	configExt      InstanceConfigExt

	status      instanceStatus
	statusGuard sync.Mutex
	exitChan    chan struct{}

	lastExit atomic.Uint32
}

func newInstance(runtime *Runtime, service *model.Service) (*instance, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	runtimeDirPath, err := util.CreateSubRuntimeDir(runtime.runtimeDirPath, fmt.Sprintf("instance-%s", id.String()))
	if err != nil {
		return nil, err
	}

	instance := &instance{
		id: id,
		//taskId:         genTaskID(service.Sname, service.Instance),
		runtimeDirPath: runtimeDirPath,
		config:         InstanceConfig{},
		configExt:      InstanceConfigExt{},
		status:         instanceStatusStopped,
		statusGuard:    sync.Mutex{},
		exitChan:       make(chan struct{}, 1),
		lastExit:       atomic.Uint32{},
	}

	if err := instance.createConfigFile(); err != nil {
		logger.ErrorLogger().Printf("rt-crosvm: Could not create config file for instance %q: %v", instance.taskId, err)
		_ = instance.close()
		return nil, err
	}

	return instance, nil
}

func (i *instance) start(runtime *Runtime) error {
	i.statusGuard.Lock()
	defer i.statusGuard.Unlock()

	if i.status == instanceStatusRunning {
		logger.WarnLogger().Printf("rt-crosvm: Ignoring instance start for %q, because it is already running", i.taskId)
		return nil
	}

	if i.status == instanceStatusClosed {
		logger.ErrorLogger().Printf("rt-crosvm: Ignoring instance start for %q, because it is already closed", i.taskId)
		return errAlreadyClosed
	}

	// clear exitChan, if neither stop nor close was called since the last start
	if len(i.exitChan) > 0 {
		_ = <-i.exitChan
	}

	runArgs := i.generateRunArgs()
	runCmd := exec.Command(runtime.executablePath, runArgs...)

	logger.InfoLogger().Printf("rt-crosvm: Starting instance %q with args %q", i.taskId, runArgs)
	if err := runCmd.Start(); err != nil {
		logger.ErrorLogger().Printf("rt-crosvm: Failed to start instance %q: %v", i.taskId, err)
		return err
	}
	logger.InfoLogger().Printf("rt-crosvm: Started instance %q", i.taskId)

	i.status = instanceStatusRunning

	go func() {
		runErr := runCmd.Wait()

		var exit instanceExit
		if runErr == nil {
			logger.InfoLogger().Printf("rt-crosvm: Instance %q exited successfully", i.taskId)
			exit = instanceExitSuccess
		} else {
			var err *exec.ExitError
			if errors.As(runErr, &err) {
				logger.ErrorLogger().Printf("rt-crosvm: Instance %q exited with error: %v", i.taskId, runErr)
			} else {
				logger.ErrorLogger().Printf("rt-crosvm: Unexpected error when trying to run instance %q: %v", i.taskId, runErr)
			}
			exit = instanceExitError
		}

		i.lastExit.Store(uint32(exit))
		i.exitChan <- struct{}{}

		i.statusGuard.Lock()
		defer i.statusGuard.Unlock()
		if i.status == instanceStatusRunning {
			i.status = instanceStatusStopped
		}
	}()

	return nil
}

func (i *instance) stop() error {
	i.statusGuard.Lock()
	defer i.statusGuard.Unlock()

	// already stopped or closed
	if i.status != instanceStatusRunning {
		return nil
	}

	if !i.callCrosvmStop() {
		logger.WarnLogger().Printf("rt-crosvm: Stopping crosvm instance %q via library failed", i.taskId)
	}

	select {
	case <-i.exitChan:
		i.status = instanceStatusStopped
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("rt-crosvm: Failed to stop crosvm instance %q", i.taskId)
	}
}

func (i *instance) close() error {
	i.statusGuard.Lock()
	defer i.statusGuard.Unlock()

	if i.status == instanceStatusClosed {
		return nil
	}

	if i.status == instanceStatusRunning {
		if !i.callCrosvmStop() {
			logger.WarnLogger().Printf("rt-crosvm: Stopping crosvm instance %q via library failed", i.taskId)
		}
		select {
		case <-i.exitChan:
			break
		case <-time.After(10 * time.Second):
			return fmt.Errorf("rt-crosvm: Failed to stop crosvm instance %q", i.taskId)
		}
	}
	i.status = instanceStatusClosed

	close(i.exitChan)
	if err := os.RemoveAll(i.runtimeDirPath); err != nil {
		logger.WarnLogger().Printf("rt-crosvm: Failed to remove runtime directory %q of instance %q: %v", i.runtimeDirPath, i.taskId, err)
	}
	return nil
}

func (i *instance) callCrosvmStop() bool {
	socketPath := path.Join(i.runtimeDirPath, socketFileName)
	return bool(C.crosvm_client_stop_vm(C.CString(socketPath)))
}

func (i *instance) createConfigFile() error {
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

func (i *instance) generateRunArgs() []string {
	return slices.Concat(
		[]string{
			"run",
			"--cfg",
			path.Join(i.runtimeDirPath, configFileName),
		},
		i.configExt.toArgs(),
	)
}
