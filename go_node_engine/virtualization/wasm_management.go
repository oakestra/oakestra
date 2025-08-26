package virtualization

import (
	"errors"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/utils"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/opencontainers/cgroups"
	"github.com/opencontainers/cgroups/fs"
)

type WasmRuntime struct {
	killQueue               map[string]chan bool
	doneQueue               map[string]chan bool
	channelLock             *sync.RWMutex
	migrationCandidates     map[string]bool
	migrationCandidatesLock *sync.RWMutex
}

var wasmRuntime = WasmRuntime{
	channelLock:             &sync.RWMutex{},
	migrationCandidates:     make(map[string]bool),
	killQueue:               make(map[string]chan bool),
	doneQueue:               make(map[string]chan bool),
	migrationCandidatesLock: &sync.RWMutex{},
}

var wasmSingletonOnce sync.Once

const (
	wasmModuleExtension   = ".wasm"
	runningAppPath        = "/etc/oakestra/wasm/running/"
	downloadedModulesPath = "/etc/oakestra/wasm/downloads/"
)

func GetWasmRuntime() Runtime {
	logger.InfoLogger().Print("Getting WASM runtime")
	wasmSingletonOnce.Do(func() {
		if _, err := os.Stat(downloadedModulesPath); os.IsNotExist(err) {
			err = os.MkdirAll(downloadedModulesPath, 0755)
			if err != nil {
				logger.ErrorLogger().Printf("Unable to create downloaded modules path: %v", err)
			}
		}
		if _, err := os.Stat(runningAppPath); os.IsNotExist(err) {
			err = os.MkdirAll(runningAppPath, 0755)
			if err != nil {
				logger.ErrorLogger().Printf("Unable to create running app path: %v", err)
			}
		}

		wasmRuntime.Cleanup()

		logger.InfoLogger().Print("WASM runtime initialized")
		model.GetNodeInfo().AddSupportedTechnology(model.WASM_RUNTIME)
	})
	return &wasmRuntime
}

func (r *WasmRuntime) Stop() {
	logger.InfoLogger().Print("Stopping WASM runtime")
	r.channelLock.Lock()
	taskIDs := reflect.ValueOf(r.killQueue).MapKeys()
	r.channelLock.Unlock()

	for _, taskid := range taskIDs {
		err := r.Undeploy(extractSnameFromTaskID(taskid.String()), extractInstanceNumberFromTaskID(taskid.String()))
		if err != nil {
			logger.ErrorLogger().Printf("Unable to undeploy %s, error: %v", taskid.String(), err)
		}
	}
	logger.InfoLogger().Print("WASM runtime stopped")
}

// prepareRuntimeEnvironment downloads and sets up the runtime environment for a WASM service
func (r *WasmRuntime) prepareRuntimeEnvironment(service model.Service) (string, string, error) {
	taskID := genTaskID(service.Sname, service.Instance)
	runtimePath := runningAppPath + taskID

	// Extract computation name
	imageSplit := strings.Split(service.Image, "/")
	if len(imageSplit) == 0 {
		return "", "", fmt.Errorf("invalid image format for service %s", taskID)
	}
	computationName := imageSplit[len(imageSplit)-1]

	// Sanitize computation name from special characters
	computationName = strings.ReplaceAll(computationName, ":", "_")
	computationName = strings.ReplaceAll(computationName, "/", "_")
	computationName = strings.ReplaceAll(computationName, ".", "_")
	computationName = strings.ReplaceAll(computationName, "?alt=media", "")

	// Check if module already downloaded otherwise download it
	compPath := downloadedModulesPath + computationName
	if _, err := os.Stat(compPath); err == nil {
		logger.InfoLogger().Printf("Module already downloaded: %s", computationName)
	} else if os.IsNotExist(err) {
		tmpCompPath, err := downloadWasmModule(service.Image)
		if err != nil {
			return "", "", fmt.Errorf("error downloading module: %v", err)
		}
		// Move from tmpCompPath to downloadedModulesPath
		err = os.Rename(tmpCompPath, downloadedModulesPath+computationName)
		if err != nil {
			return "", "", fmt.Errorf("error moving module: %v", err)
		}
	}

	// Create running app path if it does not exist
	if _, err := os.Stat(runtimePath); os.IsNotExist(err) {
		err = os.MkdirAll(runtimePath, 0755)
		if err != nil {
			return "", "", fmt.Errorf("error creating runtime path: %v", err)
		}
	}

	// Create a link to the downloaded module in the running app path
	codePath := runtimePath + "/" + computationName
	if _, err := os.Stat(codePath); os.IsNotExist(err) {
		err = os.Link(downloadedModulesPath+computationName, codePath)
		if err != nil {
			return "", "", fmt.Errorf("error linking module: %v", err)
		}
		logger.InfoLogger().Printf("Module linked to running app path: %s", codePath)
	} else {
		logger.InfoLogger().Printf("Module already exists in running app path: %s", codePath)
	}

	// Create IPC and memory files
	ipcpath := runtimePath + "/ipc"
	mainmempath := runtimePath + "/main_memory.b"
	checkpointmempath := runtimePath + "/checkpoint_memory.b"
	os.Create(ipcpath)
	os.Create(mainmempath)
	os.Create(checkpointmempath)

	return runtimePath, codePath, nil
}

func (r *WasmRuntime) Deploy(service model.Service, statusChangeNotificationHandler func(service model.Service)) error {
	taskID := genTaskID(service.Sname, service.Instance)

	r.channelLock.Lock()
	if _, serviceFound := r.killQueue[taskID]; serviceFound {
		r.channelLock.Unlock()
		return errors.New("Service already deployed")
	}
	r.channelLock.Unlock()

	logger.InfoLogger().Print("Deploying WASM service...")

	// Prepare runtime environment
	runtimePath, codePath, err := r.prepareRuntimeEnvironment(service)
	if err != nil {
		// Clean up runtime directory on failure
		if removeErr := os.RemoveAll(runtimePath); removeErr != nil {
			logger.ErrorLogger().Printf("Error cleaning up runtime directory %s: %v", runtimePath, removeErr)
		}
		return err
	}

	killChannel := make(chan bool)
	doneChannel := make(chan bool)
	startupChannel := make(chan bool, 1)
	errorChannel := make(chan error, 1)

	r.channelLock.Lock()
	r.killQueue[taskID] = killChannel
	r.doneQueue[taskID] = doneChannel
	r.channelLock.Unlock()

	go r.wasmRuntimeStartRoutine(service, killChannel, doneChannel, startupChannel, errorChannel, statusChangeNotificationHandler, codePath, false)

	success := <-startupChannel
	if !success {
		err := <-errorChannel
		return err
	}

	return nil
}

func (r *WasmRuntime) Cleanup() {
	files, err := os.ReadDir(runningAppPath)
	if err != nil {
		logger.ErrorLogger().Printf("Unable to read running app path: %v", err)
		return
	}
	for _, file := range files {
		logger.InfoLogger().Printf("Cleaning up running app: %s", file.Name())
		r.killWasmComputation(file.Name())

		// Clean up the runtime directory
		runtimePath := runningAppPath + file.Name()
		if err := os.RemoveAll(runtimePath); err != nil {
			logger.ErrorLogger().Printf("Error removing runtime directory %s: %v", runtimePath, err)
		}
	}
}

func (r *WasmRuntime) Undeploy(service string, instance int) error {
	taskID := genTaskID(service, instance)

	r.channelLock.RLock()
	killChannel, foundKill := r.killQueue[taskID]
	doneChannel, foundDone := r.doneQueue[taskID]
	r.channelLock.RUnlock()

	if foundKill && foundDone {
		logger.InfoLogger().Printf("Sending kill signal to %s", taskID)
		killChannel <- true
		select {
		case <-doneChannel:
			logger.InfoLogger().Printf("Service %s stopped", taskID)
		case <-time.After(5 * time.Second):
			logger.ErrorLogger().Printf("Timeout while stopping service %s", taskID)
		}
		r.channelLock.Lock()
		delete(r.killQueue, taskID)
		delete(r.doneQueue, taskID)
		r.channelLock.Unlock()
		return nil
	}
	return errors.New("service not found")
}

func (r *WasmRuntime) killWasmComputation(taskID string) error {
	// Clean up the runtime directory first
	runtimePath := runningAppPath + taskID
	defer func() {
		if err := os.RemoveAll(runtimePath); err != nil {
			logger.ErrorLogger().Printf("Error removing runtime directory %s: %v", runtimePath, err)
		}
	}()

	cgroupsPath := fmt.Sprintf("%s/%s", NAMESPACE, taskID)
	if err := os.RemoveAll(cgroupsPath); err != nil {
		return fmt.Errorf("error removing cgroup %s: %v", cgroupsPath, err)
	}

	statsManager, err := getCgroupStatsManager(cgroupsPath)
	if err != nil {
		return fmt.Errorf("error getting cgroup stats manager for task %s: %v", taskID, err)
	}

	pids, err := statsManager.GetAllPids()
	if err != nil {
		return fmt.Errorf("error getting all PIDs for task %s: %v", taskID, err)
	}

	if len(pids) == 0 {
		return fmt.Errorf("no PIDs found for task %s", taskID)
	}

	for _, pid := range pids {
		//kill pid
		process, err := os.FindProcess(pid)
		if err != nil {
			logger.ErrorLogger().Printf("Error finding process %d for task %s: %v", pid, taskID, err)
			continue
		}
		if err := process.Kill(); err != nil {
			logger.ErrorLogger().Printf("Error killing process %d for task %s: %v", pid, taskID, err)
		}
	}

	_ = statsManager.Destroy()

	return nil
}

func (r *WasmRuntime) wasmRuntimeStartRoutine(
	service model.Service,
	killChannel chan bool,
	doneChannel chan bool,
	startup chan bool,
	errorchan chan error,
	statusChangeNotificationHandler func(service model.Service),
	codePath string,
	isResuming bool,
) {
	taskID := genTaskID(service.Sname, service.Instance)
	service.Status = model.SERVICE_CREATED
	statusChangeNotificationHandler(service)

	// Create runtime path early for cleanup purposes
	runtimePath := runningAppPath + taskID

	revert := func(err error) {
		startup <- false
		errorchan <- err
		r.channelLock.Lock()
		delete(r.killQueue, taskID)
		delete(r.doneQueue, taskID)
		r.channelLock.Unlock()

		// Clean up runtime directory on failure
		if removeErr := os.RemoveAll(runtimePath); removeErr != nil {
			logger.ErrorLogger().Printf("Error cleaning up runtime directory %s: %v", runtimePath, removeErr)
		}
	}

	// Create IPC and memory file paths
	ipcpath := runtimePath + "/ipc"
	mainmempath := runtimePath + "/main_memory.b"
	checkpointmempath := runtimePath + "/checkpoint_memory.b"

	// Execute create_command - this works for both new deployments and resuming from state
	// The command creates/starts a computation directly inside the oakestra namespace
	cmd := exec.Command("/etc/oakestra/wasm/create_command", codePath, ipcpath, mainmempath, checkpointmempath, taskID)
	cmd.Dir = runtimePath
	if err := cmd.Run(); err != nil {
		revert(fmt.Errorf("error executing create command: %v", err))
		return
	}

	//attach network
	//TODO

	cmd = exec.Command("/etc/oakestra/wasm/start_command", ipcpath)
	cmd.Dir = runtimePath
	if err := cmd.Run(); err != nil {
		revert(fmt.Errorf("error executing create command: %v", err))
		return
	}

	// Notify successful startup
	startup <- true
	service.Status = model.SERVICE_RUNNING
	statusChangeNotificationHandler(service)

	select {
	case <-killChannel:
		logger.InfoLogger().Printf("Kill channel message received for WASM module %s", taskID)
		r.killWasmComputation(taskID)
		service.Status = model.SERVICE_DEAD
		statusChangeNotificationHandler(service)
	}

	doneChannel <- true
	r.channelLock.Lock()
	delete(r.killQueue, taskID)
	delete(r.doneQueue, taskID)
	r.channelLock.Unlock()
}

func (r *WasmRuntime) ResourceMonitoring(every time.Duration, notifyHandler func(res []model.Resources)) {
	for {
		time.Sleep(every)
		resourceList := []model.Resources{}
		r.channelLock.RLock()
		for taskid := range r.killQueue {
			// get resource usage from cgroup with taskid
			cgroupPath := fmt.Sprintf("/sys/fs/cgroup/%s/%s", NAMESPACE, taskid)

			statsManager, err := getCgroupStatsManager(cgroupPath)
			if err != nil {
				logger.ErrorLogger().Printf("Error getting cgroup stats manager for task %s: %v", taskid, err)
				continue
			}
			stats, err := statsManager.GetStats()
			if err != nil {
				logger.ErrorLogger().Printf("Error getting cgroup stats for task %s: %v", taskid, err)
				continue
			}

			// get resource consumption from cgroupPath
			resourceList = append(resourceList, model.Resources{
				Cpu:      fmt.Sprintf("%f", float64(stats.CpuStats.CpuUsage.TotalUsage)),
				Memory:   fmt.Sprintf("%f", float64(stats.MemoryStats.Usage.Usage)),
				Disk:     "0",
				Sname:    extractSnameFromTaskID(taskid),
				Logs:     getLogs(taskid),
				Runtime:  string(model.WASM_RUNTIME),
				Instance: extractInstanceNumberFromTaskID(taskid),
			})
		}
		r.channelLock.RUnlock()
		notifyHandler(resourceList)
	}
}

func getCgroupStatsManager(path string) (*fs.Manager, error) {
	statsManager, err := fs.NewManager(
		&cgroups.Cgroup{
			Path: path,
		},
		nil)
	if err != nil {
		return nil, err
	}
	return statsManager, nil
}

// SetMigrationCandidate checks if the service can be migrated and marks it as a candidate.
func (r *WasmRuntime) SetMigrationCandidate(sname string, instance int) (model.Service, error) {
	taskID := genTaskID(sname, instance)

	r.channelLock.RLock()
	_, serviceExists := r.killQueue[taskID]
	r.channelLock.RUnlock()

	if !serviceExists {
		return model.Service{}, fmt.Errorf("service %s instance %d is not deployed", sname, instance)
	}

	r.migrationCandidatesLock.Lock()
	defer r.migrationCandidatesLock.Unlock()

	if r.migrationCandidates[taskID] {
		return model.Service{}, fmt.Errorf("service %s instance %d is already marked as migration candidate", sname, instance)
	}

	r.migrationCandidates[taskID] = true
	logger.InfoLogger().Printf("Service %s marked as migration candidate", taskID)

	// Return the service information (we need to reconstruct it from taskID)
	service := model.Service{
		Sname:    sname,
		Instance: instance,
		Status:   model.SERVICE_MIGRATION_ACCEPTED,
	}

	return service, nil
}

// RemoveMigrationCandidate removes the migration candidate mark from a service.
func (r *WasmRuntime) RemoveMigrationCandidate(sname string, instance int) error {
	taskID := genTaskID(sname, instance)

	r.migrationCandidatesLock.Lock()
	defer r.migrationCandidatesLock.Unlock()

	if !r.migrationCandidates[taskID] {
		return fmt.Errorf("service %s instance %d is not marked as migration candidate", sname, instance)
	}

	delete(r.migrationCandidates, taskID)
	logger.InfoLogger().Printf("Service %s removed from migration candidates", taskID)

	return nil
}

// StopAndGetState stops a service and returns its state if it has been marked as a migration candidate.
func (r *WasmRuntime) StopAndGetState(sname string, instance int) (utils.OnceReader, error) {
	taskID := genTaskID(sname, instance)

	r.migrationCandidatesLock.RLock()
	isMigrationCandidate := r.migrationCandidates[taskID]
	r.migrationCandidatesLock.RUnlock()

	if !isMigrationCandidate {
		return nil, fmt.Errorf("service %s instance %d is not marked as migration candidate", sname, instance)
	}

	r.channelLock.RLock()
	killChannel, serviceExists := r.killQueue[taskID]
	doneChannel, _ := r.doneQueue[taskID]
	r.channelLock.RUnlock()

	if !serviceExists {
		return nil, fmt.Errorf("service %s instance %d is not running", sname, instance)
	}

	revertMigrationCandidate := func() {
		r.migrationCandidatesLock.Lock()
		delete(r.migrationCandidates, taskID)
		r.migrationCandidatesLock.Unlock()
	}

	// Create checkpoint before stopping
	runtimePath := runningAppPath + taskID
	stateFile := runtimePath + "/checkpoint_memory.tar.gz"

	// Execute checkpoint command - pause the WASM computation and capture state
	ipcpath := runtimePath + "/ipc"

	cmd := exec.Command("/etc/oakestra/wasm/migrate_command", ipcpath)
	cmd.Dir = runtimePath
	if err := cmd.Run(); err != nil {
		defer revertMigrationCandidate()
		return nil, fmt.Errorf("error creating checkpoint for %s: %v", taskID, err)
	}

	// Compress checkpoint memory file into tar.gz file
	// Use -C to change to runtime directory and compress just the checkpoint_memory.b file
	cmd = exec.Command("tar", "-czf", stateFile, "-C", runtimePath, "checkpoint_memory.b")
	if err := cmd.Run(); err != nil {
		defer revertMigrationCandidate()
		return nil, fmt.Errorf("error compressing checkpoint for %s: %v", taskID, err)
	}

	// Stop the service
	logger.InfoLogger().Printf("Stopping WASM service %s for migration", taskID)
	killChannel <- true

	// Wait for service to stop
	select {
	case <-doneChannel:
		logger.InfoLogger().Printf("Service %s stopped for migration", taskID)
	case <-time.After(10 * time.Second):
		defer revertMigrationCandidate()
		defer os.Remove(stateFile)
		return nil, fmt.Errorf("timeout stopping service %s for migration", taskID)
	}

	// Clean up migration candidate status
	r.migrationCandidatesLock.Lock()
	delete(r.migrationCandidates, taskID)
	r.migrationCandidatesLock.Unlock()

	// Create OnceReader for the checkpoint file
	f, err := os.Open(stateFile)
	if err != nil {
		defer os.Remove(stateFile)
		return nil, fmt.Errorf("error opening checkpoint file %s: %v", stateFile, err)
	}

	reader := utils.NewOnceReader(f)

	logger.InfoLogger().Printf("Service %s stopped and state captured for migration", taskID)
	return reader, nil
}

// PrepareForInstantiantion prepares the service for instantiation after migration.
func (r *WasmRuntime) PrepareForInstantiantion(service model.Service, statusChangeNotificationHandler func(service model.Service)) error {
	taskID := genTaskID(service.Sname, service.Instance)

	// Check if service is already running
	r.channelLock.RLock()
	_, serviceExists := r.killQueue[taskID]
	r.channelLock.RUnlock()

	if serviceExists {
		return fmt.Errorf("service %s instance %d is already running", service.Sname, service.Instance)
	}

	logger.InfoLogger().Printf("Preparing service %s for migration instantiation", taskID)

	// Update service status
	service.Status = model.SERVICE_MIGRATION_PROGRESS
	statusChangeNotificationHandler(service)

	// Prepare runtime environment using the helper function
	runtimePath, _, err := r.prepareRuntimeEnvironment(service)
	if err != nil {
		service.Status = model.SERVICE_DEAD
		statusChangeNotificationHandler(service)
		// Clean up runtime directory on failure
		if removeErr := os.RemoveAll(runtimePath); removeErr != nil {
			logger.ErrorLogger().Printf("Error cleaning up runtime directory %s: %v", runtimePath, removeErr)
		}
		return err
	}

	logger.InfoLogger().Printf("Service %s prepared for migration instantiation", taskID)
	return nil
}

// AbortMigration aborts the migration process for a service.
func (r *WasmRuntime) AbortMigration(service model.Service) error {
	taskID := genTaskID(service.Sname, service.Instance)

	logger.InfoLogger().Printf("Aborting migration for service %s", taskID)

	// Clean up runtime path
	runtimePath := runningAppPath + taskID
	if err := os.RemoveAll(runtimePath); err != nil {
		logger.ErrorLogger().Printf("Error cleaning up runtime path for %s: %v", taskID, err)
	}

	// Remove from migration candidates if present
	r.migrationCandidatesLock.Lock()
	delete(r.migrationCandidates, taskID)
	r.migrationCandidatesLock.Unlock()

	//delete running directory
	os.RemoveAll(runtimePath)

	logger.InfoLogger().Printf("Migration aborted for service %s", taskID)
	return nil
}

// ResumeFromState resumes a service from a given state after migration.
func (r *WasmRuntime) ResumeFromState(sname string, instance int, stateFile string, statusChangeNotificationHandler func(service model.Service)) error {
	taskID := genTaskID(sname, instance)

	// Check if service is already running
	r.channelLock.RLock()
	_, serviceExists := r.killQueue[taskID]
	r.channelLock.RUnlock()

	if serviceExists {
		return fmt.Errorf("service %s instance %d is already running", sname, instance)
	}

	logger.InfoLogger().Printf("Resuming service %s from state file %s", taskID, stateFile)

	// Remove the state file after function execution
	defer func() {
		if err := os.Remove(stateFile); err != nil {
			logger.ErrorLogger().Printf("Unable to remove state file %s: %v", stateFile, err)
		}
	}()

	// Get runtime path
	runtimePath := runningAppPath + taskID

	// Extract checkpoint memory file from tar.gz directly to the runtime directory
	cmd := exec.Command("tar", "-xzf", stateFile, "-C", runtimePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error extracting state file %s to %s: %v", stateFile, runtimePath, err)
	}

	// Create channels for the resumed service
	killChannel := make(chan bool)
	doneChannel := make(chan bool)
	startupChannel := make(chan bool, 1)
	errorChannel := make(chan error, 1)

	r.channelLock.Lock()
	r.killQueue[taskID] = killChannel
	r.doneQueue[taskID] = doneChannel
	r.channelLock.Unlock()

	// Create service object for status notification
	service := model.Service{
		Sname:    sname,
		Instance: instance,
		Status:   model.SERVICE_RUNNING,
	}

	// Find the code path (WASM module) in the runtime directory
	files, err := os.ReadDir(runtimePath)
	if err != nil {
		return fmt.Errorf("error reading runtime directory %s: %v", runtimePath, err)
	}

	var codePath string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), wasmModuleExtension) {
			codePath = runtimePath + "/" + file.Name()
			break
		}
	}

	if codePath == "" {
		return fmt.Errorf("no WASM module found in runtime directory %s", runtimePath)
	}

	// Start the WASM runtime creation routine
	go r.wasmRuntimeStartRoutine(service, killChannel, doneChannel, startupChannel, errorChannel, statusChangeNotificationHandler, codePath, true)

	// Wait for startup
	success := <-startupChannel
	if !success {
		err := <-errorChannel
		return err
	}

	logger.InfoLogger().Printf("Service %s resumed from migration state", taskID)
	return nil
}
