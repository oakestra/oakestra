package virtualization

import (
	"errors"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/requests"
	"go_node_engine/utils"
	"io/ioutil"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opencontainers/cgroups"
	"github.com/opencontainers/cgroups/fs"
	"github.com/vishvananda/netns"
)

type WasmService struct {
	service *model.Service
	kill    chan bool
	done    chan bool
}

type WasmRuntime struct {
	services    map[string]*WasmService
	channelLock *sync.RWMutex
}

var wasmRuntime = WasmRuntime{
	channelLock: &sync.RWMutex{},
	services:    make(map[string]*WasmService),
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
	taskIDs := reflect.ValueOf(r.services).MapKeys()
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
	s, serviceFound := r.services[taskID]
	if serviceFound && s != nil {
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
	s = &WasmService{
		service: &service,
		kill:    killChannel,
		done:    doneChannel,
	}
	r.services[taskID] = s
	r.channelLock.Unlock()

	go r.wasmRuntimeStartRoutine(s, killChannel, doneChannel, startupChannel, errorChannel, statusChangeNotificationHandler, codePath)

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
	s, found := r.services[taskID]
	r.channelLock.RUnlock()

	if found && s != nil {
		logger.InfoLogger().Printf("Sending kill signal to %s", taskID)
		s.kill <- true

		select {
		case <-s.done:
			logger.InfoLogger().Printf("Service %s stopped", taskID)
		case <-time.After(5 * time.Second):
			logger.ErrorLogger().Printf("Timeout while stopping service %s", taskID)
			// TODO: Force kill if necessary
		}
		return nil
	}
	return errors.New("service not found")
}

func (r *WasmRuntime) killWasmComputation(taskID string) error {
	s, exists := r.services[taskID]

	//cleanup service list
	if exists && s != nil {
		r.channelLock.Lock()
		delete(r.services, taskID)
		r.channelLock.Unlock()
	}

	// Clean up the runtime directory first
	runtimePath := runningAppPath + taskID
	defer func() {
		if err := os.RemoveAll(runtimePath); err != nil {
			logger.ErrorLogger().Printf("Error removing runtime directory %s: %v", runtimePath, err)
		}
	}()

	// Remove the cgroup and kill all processes in it
	if err := r.removeCgroup(taskID); err != nil {
		logger.ErrorLogger().Printf("Error removing cgroup for task %s: %v", taskID, err)
		// Don't return error here, continue with cleanup
	}

	// Remove netns
	if err := netns.DeleteNamed(taskID); err != nil {
		logger.ErrorLogger().Printf("Error removing netns for task %s: %v", taskID, err)
		// Don't return error here, continue with cleanup
	}

	return nil
}

func (r *WasmRuntime) wasmRuntimeStartRoutine(
	s *WasmService,
	killChannel chan bool,
	doneChannel chan bool,
	startup chan bool,
	errorchan chan error,
	statusChangeNotificationHandler func(service model.Service),
	codePath string,
) {
	service := s.service
	taskID := genTaskID(service.Sname, service.Instance)
	service.Status = model.SERVICE_CREATED
	statusChangeNotificationHandler(*service)

	// Create runtime path early for cleanup purposes
	runtimePath := runningAppPath + taskID

	revert := func(err error) {
		startup <- false
		errorchan <- err
		r.channelLock.Lock()
		delete(r.services, taskID)
		r.channelLock.Unlock()

		// Clean up cgroup on failure
		if cgroupErr := r.removeCgroup(taskID); cgroupErr != nil {
			logger.ErrorLogger().Printf("Error cleaning up cgroup for %s: %v", taskID, cgroupErr)
		}

		// Clean up runtime directory on failure
		if removeErr := os.RemoveAll(runtimePath); removeErr != nil {
			logger.ErrorLogger().Printf("Error cleaning up runtime directory %s: %v", runtimePath, removeErr)
		}
	}

	// Create IPC and memory file paths
	ipcpath := runtimePath + "/ipc"
	mainmempath := runtimePath + "/main_memory.b"
	checkpointmempath := runtimePath + "/checkpoint_memory.b"
	logpath := model.GetNodeInfo().LogDirectory + "/" + taskID

	// Create cgroup v2 for the task
	_, err := r.createCgroup(taskID)
	if err != nil {
		revert(err)
		return
	}

	// Execute create_command - this works for both new deployments and resuming from state
	// The command creates/starts a computation directly inside the oakestra namespace
	taskpid, err := wasmCreateCommand(codePath, ipcpath, mainmempath, checkpointmempath, runtimePath, taskID, logpath)
	if err != nil {
		revert(err)
		return
	}

	exitChannel := make(chan bool, 1)
	go func(pid int) {
		procPath := fmt.Sprintf("/proc/%d", pid)
		for {
			time.Sleep(1 * time.Second)
			if _, err := os.Stat(procPath); os.IsNotExist(err) {
				exitChannel <- true
				return
			}
		}
	}(taskpid)

	//attach network if node in overlay network mode
	if model.GetNodeInfo().Overlay {
		err = requests.AttachNetworkToTask(taskpid, service.Sname, service.Instance, service.Ports, requests.NETWORK_TYPE_WASM)
		if err != nil {
			logger.ErrorLogger().Printf("Unable to attach network interface to the task: %v", err)
			revert(err)
			return
		}
	}

	cmd := exec.Command("/etc/oakestra/wasm/start_command", ipcpath)
	cmd.Dir = runtimePath
	if err := cmd.Run(); err != nil {
		revert(fmt.Errorf("error executing create command: %v", err))
		return
	}

	// Notify successful startup
	startup <- true
	service.Status = model.SERVICE_RUNNING
	statusChangeNotificationHandler(*service)

	select {
	case <-exitChannel:
		logger.InfoLogger().Printf("WASM module %s has exited", taskID)
		service.Status = model.SERVICE_FAILED
		if service.OneShot {
			// If this is a one-shot service, mark it as completed
			service.Status = model.SERVICE_COMPLETED
		}
		statusChangeNotificationHandler(*service)
	case <-killChannel:
		logger.InfoLogger().Printf("Kill channel message received for WASM module %s", taskID)
		service.Status = model.SERVICE_DEAD
		statusChangeNotificationHandler(*service)
	}

	//detaching network
	if model.GetNodeInfo().Overlay {
		_ = requests.DetachNetworkFromTask(service.Sname, service.Instance, requests.NETWORK_TYPE_WASM)
	}

	r.killWasmComputation(taskID)
}

func (r *WasmRuntime) ResourceMonitoring(every time.Duration, notifyHandler func(res []model.Resources)) {
	for {
		time.Sleep(every)
		resourceList := []model.Resources{}
		r.channelLock.RLock()
		for taskid := range r.services {
			if r.services[taskid] != nil {

				s := r.services[taskid]

				// get resource usage from cgroup with taskid
				cgroupPath := fmt.Sprintf("/sys/fs/cgroup/%s/%s", NAMESPACE, taskid)

				// Use the new cgroup v2 stats reading
				stats, err := getCgroupV2Stats(cgroupPath)
				if err != nil {
					logger.ErrorLogger().Printf("Error getting cgroup v2 stats for task %s: %v", taskid, err)
					continue
				}

				// get resource consumption from cgroupPath
				resourceList = append(resourceList, model.Resources{
					Cpu:      fmt.Sprintf("%d", stats.CPUUsage),
					Memory:   fmt.Sprintf("%d", stats.MemoryUsage),
					Disk:     "0",
					Sname:    extractSnameFromTaskID(taskid),
					Logs:     getLogs(taskid),
					Runtime:  string(model.WASM_RUNTIME),
					Instance: extractInstanceNumberFromTaskID(taskid),
					Status:   s.service.Status,
				})

			}
		}
		r.channelLock.RUnlock()
		notifyHandler(resourceList)
	}
}

// createCgroup creates a cgroup v2 directory structure and initializes it
func (r *WasmRuntime) createCgroup(taskID string) (string, error) {
	cgroupPath := fmt.Sprintf("/sys/fs/cgroup/%s/%s", NAMESPACE, taskID)

	// Create the cgroup directory structure
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return "", fmt.Errorf("error creating cgroup directory %s: %v", cgroupPath, err)
	}

	// Initialize the cgroup by writing to cgroup.procs (this creates the cgroup)
	// We don't add any processes yet, but this ensures the cgroup is properly initialized
	procsFile := cgroupPath + "/cgroup.procs"
	if _, err := os.OpenFile(procsFile, os.O_WRONLY|os.O_CREATE, 0644); err != nil {
		return "", fmt.Errorf("error initializing cgroup procs file %s: %v", procsFile, err)
	}

	return cgroupPath, nil
}

// removeCgroup removes a cgroup after killing all processes in it
func (r *WasmRuntime) removeCgroup(taskID string) error {
	cgroupPath := fmt.Sprintf("/sys/fs/cgroup/%s/%s", NAMESPACE, taskID)

	// Get PIDs directly from cgroup v2 filesystem
	stats, err := getCgroupV2Stats(cgroupPath)
	if err != nil {
		logger.ErrorLogger().Printf("Error reading cgroup v2 stats for task %s: %v", taskID, err)
		// Continue with cleanup even if we can't read stats
	} else {
		// Kill all processes in the cgroup
		for _, pid := range stats.PIDs {
			process, err := os.FindProcess(pid)
			if err != nil {
				logger.ErrorLogger().Printf("Error finding process %d for task %s: %v", pid, taskID, err)
				continue
			}
			if err := process.Kill(); err != nil {
				logger.ErrorLogger().Printf("Error killing process %d for task %s: %v", pid, taskID, err)
			}
		}
	}

	// Remove the cgroup directory
	if err := os.RemoveAll(cgroupPath); err != nil {
		return fmt.Errorf("error removing cgroup directory %s: %v", cgroupPath, err)
	}

	//remove network
	//detaching network
	if model.GetNodeInfo().Overlay {
		go requests.DetachNetworkFromTask(extractSnameFromTaskID(taskID), extractInstanceNumberFromTaskID(taskID), requests.NETWORK_TYPE_WASM)
	}

	logger.InfoLogger().Printf("Cgroup %s removed successfully", cgroupPath)
	return nil
} // Simple cgroup v2 stats structure
type CgroupV2Stats struct {
	CPUUsage    uint64
	MemoryUsage uint64
	PIDs        []int
}

// getCgroupV2Stats reads stats directly from cgroup v2 filesystem
func getCgroupV2Stats(cgroupPath string) (*CgroupV2Stats, error) {
	stats := &CgroupV2Stats{}

	// Read CPU stats
	cpuStatPath := cgroupPath + "/cpu.stat"
	if cpuData, err := ioutil.ReadFile(cpuStatPath); err == nil {
		lines := strings.Split(string(cpuData), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "usage_usec ") {
				parts := strings.Fields(line)
				if len(parts) == 2 {
					if usage, err := strconv.ParseUint(parts[1], 10, 64); err == nil {
						stats.CPUUsage = usage * 1000 // Convert microseconds to nanoseconds
					}
				}
			}
		}
	}

	// Read memory stats
	memoryCurrentPath := cgroupPath + "/memory.current"
	if memData, err := ioutil.ReadFile(memoryCurrentPath); err == nil {
		if usage, err := strconv.ParseUint(strings.TrimSpace(string(memData)), 10, 64); err == nil {
			stats.MemoryUsage = usage
		}
	}

	// Read PIDs
	procsPath := cgroupPath + "/cgroup.procs"
	if procData, err := ioutil.ReadFile(procsPath); err == nil {
		lines := strings.Split(strings.TrimSpace(string(procData)), "\n")
		for _, line := range lines {
			if line != "" {
				if pid, err := strconv.Atoi(line); err == nil {
					stats.PIDs = append(stats.PIDs, pid)
				}
			}
		}
	}

	if len(stats.PIDs) == 0 {
		return nil, fmt.Errorf("no PIDs found in cgroup %s", cgroupPath)
	}

	return stats, nil
}

func getCgroupStatsManager(path string) (*fs.Manager, error) {
	// This function is kept for compatibility with existing code
	// but we'll use the new getCgroupV2Stats function for actual stats reading
	statsManager, err := fs.NewManager(
		&cgroups.Cgroup{
			Path: path,
		},
		map[string]string{})
	if err != nil {
		return nil, err
	}
	return statsManager, nil
}

// SetMigrationCandidate checks if the service can be migrated and marks it as a candidate.
func (r *WasmRuntime) SetMigrationCandidate(sname string, instance int) (model.Service, error) {
	taskID := genTaskID(sname, instance)

	r.channelLock.RLock()
	s, serviceExists := r.services[taskID]
	r.channelLock.RUnlock()

	if serviceExists && s == nil {
		return model.Service{}, fmt.Errorf("service %s instance %d is not deployed", sname, instance)
	}

	// check if the service is in any of the migration statuses
	if s.service.Status == model.SERVICE_MIGRATION_ACCEPTED ||
		s.service.Status == model.SERVICE_MIGRATION_PROGRESS ||
		s.service.Status == model.SERVICE_MIGRATION_REQUESTED ||
		s.service.Status == model.SERVICE_MIGRATION_DEBOUNCE {
		return model.Service{}, fmt.Errorf("service %s instance %d is already in migration process", sname, instance)
	}

	// check if service is running
	if s.service.Status != model.SERVICE_RUNNING {
		return model.Service{}, fmt.Errorf("service %s instance %d is not running, cannot mark as migration candidate", sname, instance)
	}

	// mark the service as a migration candidate
	r.channelLock.Lock()
	s.service.Status = model.SERVICE_MIGRATION_ACCEPTED
	r.channelLock.Unlock()

	logger.InfoLogger().Printf("Service %s marked as migration candidate", taskID)

	return *s.service, nil
}

// RemoveMigrationCandidate removes the migration candidate mark from a service.
func (r *WasmRuntime) RemoveMigrationCandidate(sname string, instance int) error {
	taskID := genTaskID(sname, instance)

	r.channelLock.Lock()
	s, exists := r.services[taskID]
	r.channelLock.Unlock()

	if !exists || s == nil {
		return fmt.Errorf("service %s instance %d is not deployed", sname, instance)
	}

	// check if the service is in any of the migration statuses
	if s.service.Status == model.SERVICE_MIGRATION_ACCEPTED {
		r.channelLock.Lock()
		s.service.Status = model.SERVICE_RUNNING
		r.channelLock.Unlock()
	} else {
		return fmt.Errorf("service %s instance %d is not marked as a migration candidate", sname, instance)
	}

	return nil
}

// StopAndGetState stops a service and returns its state if it has been marked as a migration candidate.
func (r *WasmRuntime) StopAndGetState(sname string, instance int) (utils.OnceReader, error) {
	taskID := genTaskID(sname, instance)

	r.channelLock.RLock()
	s, exists := r.services[taskID]
	r.channelLock.RUnlock()

	if exists && s == nil {
		return nil, fmt.Errorf("service %s instance %d is not running", sname, instance)
	}

	// check if the service is in any of the migration statuses
	if s.service.Status != model.SERVICE_MIGRATION_ACCEPTED {
		return nil, fmt.Errorf("service %s instance %d is not marked as a migration candidate", sname, instance)
	}

	r.channelLock.Lock()
	s.service.Status = model.SERVICE_MIGRATION_PROGRESS
	r.channelLock.Unlock()

	revertState := func() {
		r.channelLock.Lock()
		s.service.Status = model.SERVICE_RUNNING
		r.channelLock.Unlock()
	}

	// Create checkpoint before stopping
	runtimePath := runningAppPath + taskID
	stateFile := runtimePath + "/checkpoint_memory.tar.gz"

	// Execute checkpoint command - pause the WASM computation and capture state
	ipcpath := runtimePath + "/ipc"

	cmd := exec.Command("/etc/oakestra/wasm/migrate_command", ipcpath)
	cmd.Dir = runtimePath
	if err := cmd.Run(); err != nil {
		defer revertState()
		return nil, fmt.Errorf("error creating checkpoint for %s: %v", taskID, err)
	}

	// Compress checkpoint memory file into tar.gz file
	// Use -C to change to runtime directory and compress just the checkpoint_memory.b file
	cmd = exec.Command("tar", "-czf", stateFile, "-C", runtimePath, "checkpoint_memory.b")
	if err := cmd.Run(); err != nil {
		defer revertState()
		return nil, fmt.Errorf("error compressing checkpoint for %s: %v", taskID, err)
	}

	// Create OnceReader for the checkpoint file
	f, err := os.Open(stateFile)
	if err != nil {
		defer os.Remove(stateFile)
		defer revertState()
		return nil, fmt.Errorf("error opening checkpoint file %s: %v", stateFile, err)
	}
	reader := utils.NewOnceReader(f)

	// Stop the service
	logger.InfoLogger().Printf("Stopping WASM service %s for migration", taskID)
	err = r.Undeploy(extractSnameFromTaskID(taskID), extractInstanceNumberFromTaskID(taskID))
	if err != nil {
		defer revertState()
		return nil, fmt.Errorf("error stopping service %s: %v", taskID, err)
	}

	logger.InfoLogger().Printf("Service %s stopped and state captured for migration", taskID)
	return reader, nil
}

// PrepareForInstantiantion prepares the service for instantiation after migration.
func (r *WasmRuntime) PrepareForInstantiantion(service model.Service, statusChangeNotificationHandler func(service model.Service)) error {
	taskID := genTaskID(service.Sname, service.Instance)

	// Check if service is already running
	r.channelLock.RLock()
	s, exists := r.services[taskID]
	r.channelLock.RUnlock()

	if exists && s == nil {
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
	r.killWasmComputation(taskID)

	logger.InfoLogger().Printf("Migration aborted for service %s", taskID)
	return nil
}

// ResumeFromState resumes a service from a given state after migration.
func (r *WasmRuntime) ResumeFromState(sname string, instance int, stateFile string, statusChangeNotificationHandler func(service model.Service)) error {
	taskID := genTaskID(sname, instance)

	// Check if service is already running
	r.channelLock.RLock()
	s, exists := r.services[taskID]
	r.channelLock.RUnlock()

	if exists && s == nil {
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
	s.kill = killChannel
	s.done = doneChannel
	r.channelLock.Unlock()

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
	go r.wasmRuntimeStartRoutine(s, killChannel, doneChannel, startupChannel, errorChannel, statusChangeNotificationHandler, codePath)

	// Wait for startup
	success := <-startupChannel
	if !success {
		err := <-errorChannel
		return err
	}

	logger.InfoLogger().Printf("Service %s resumed from migration state", taskID)
	return nil
}

// performs the wasm create command
// returns the PID or an error
func wasmCreateCommand(codePath, ipcpath, mainmempath, checkpointmempath, runtimePath, taskID string, logpath string) (int, error) {
	err := utils.CreateNetnsByName(taskID)
	if err != nil {
		return 0, fmt.Errorf("error creating network namespace: %v", err)
	}

	taskPID := 0

	err = utils.ExecInsideNsByName(taskID, func() error {
		cmd := exec.Command("nohup", "/etc/oakestra/wasm/create_command", codePath, ipcpath, mainmempath, checkpointmempath, taskID, logpath, ">>", logpath, "2>&1")
		cmd.Dir = runtimePath
		cmd.Stdin = nil
		err := cmd.Run()
		if err != nil {
			fmt.Println(err)
			//return 0, fmt.Errorf("error executing create command: %v", err)
		}

		//get PID from cgroup filesystem
		cgroupPath := fmt.Sprintf("/sys/fs/cgroup/%s/%s", NAMESPACE, taskID)
		procsFile := cgroupPath + "/cgroup.procs"
		data, err := os.ReadFile(procsFile)
		if err != nil {
			return fmt.Errorf("error reading cgroup procs file %s: %v", procsFile, err)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) == 0 {
			return fmt.Errorf("no PID found in cgroup procs file %s", procsFile)
		}
		taskPID, err = strconv.Atoi(lines[0])
		if err != nil {
			return fmt.Errorf("error converting PID to integer: %v", err)
		}

		if taskPID == 0 {
			return fmt.Errorf("No PID returned from create command")
		}

		return nil
	})
	return taskPID, nil
}
