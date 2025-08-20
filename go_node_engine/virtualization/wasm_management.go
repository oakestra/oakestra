package virtualization

import (
	"errors"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
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
	killQueue   map[string]chan bool
	doneQueue   map[string]chan bool
	channelLock *sync.RWMutex
}

var wasmRuntime = WasmRuntime{
	channelLock: &sync.RWMutex{},
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
		wasmRuntime.killQueue = make(map[string]chan bool)
		wasmRuntime.doneQueue = make(map[string]chan bool)
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

func (r *WasmRuntime) Deploy(service model.Service, statusChangeNotificationHandler func(service model.Service)) error {

	killChannel := make(chan bool)
	doneChannel := make(chan bool)
	startupChannel := make(chan bool, 1)
	errorChannel := make(chan error, 1)

	taskID := genTaskID(service.Sname, service.Instance)

	r.channelLock.Lock()
	if _, serviceFound := r.killQueue[taskID]; serviceFound {
		r.channelLock.Unlock()
		return errors.New("Service already deployed")
	}
	r.killQueue[taskID] = killChannel
	r.doneQueue[taskID] = doneChannel
	r.channelLock.Unlock()

	logger.InfoLogger().Print("Deploying WASM service...")
	go r.wasmRuntimeCreationRoutine(service, killChannel, doneChannel, startupChannel, errorChannel, statusChangeNotificationHandler)

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
	}
	for _, file := range files {
		logger.InfoLogger().Printf("Cleaning up running app: %s", file.Name())
		r.killWasmComputation(file.Name())
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
	cgroupsPath := fmt.Sprintf("%s/%d", NAMESPACE, taskID)
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

func (r *WasmRuntime) wasmRuntimeCreationRoutine(
	service model.Service,
	killChannel chan bool,
	doneChannel chan bool,
	startup chan bool,
	errorchan chan error,
	statusChangeNotificationHandler func(service model.Service),
) {
	taskID := genTaskID(service.Sname, service.Instance)
	service.Status = model.SERVICE_CREATED
	statusChangeNotificationHandler(service)

	revert := func(err error) {
		startup <- false
		errorchan <- err
		r.channelLock.Lock()
		delete(r.killQueue, taskID)
		delete(r.doneQueue, taskID)
		r.channelLock.Unlock()
	}

	// Extract computation name
	imageSplit := strings.Split(service.Image, "/")
	if len(imageSplit) == 0 {
		revert(fmt.Errorf("invalid image format for service %s", taskID))
		return
	}
	computationName := imageSplit[len(imageSplit)-1]

	//sanitize computation name from special characters
	computationName = strings.ReplaceAll(computationName, ":", "_")
	computationName = strings.ReplaceAll(computationName, "/", "_")
	computationName = strings.ReplaceAll(computationName, ".", "_")

	//Check if module already downloaded otherwise download it
	compPath := downloadedModulesPath + computationName
	if _, err := os.Stat(compPath); err == nil {
		logger.InfoLogger().Printf("Module already downloaded: %s", computationName)
	} else if os.IsNotExist(err) {
		tmpCompPath, err := downloadWasmModule(service.Image)
		if err != nil {
			revert(fmt.Errorf("error downloading module: %v", err))
			return
		}
		//move from tmpCompPath to downloadedModulesPath
		err = os.Rename(tmpCompPath, downloadedModulesPath+computationName)
		if err != nil {
			revert(fmt.Errorf("error moving module: %v", err))
			return
		}
	}

	// Create running app path if it does not exist
	runtimePath := runningAppPath + taskID
	if _, err := os.Stat(runtimePath); os.IsNotExist(err) {
		err = os.MkdirAll(runtimePath, 0755)
		if err != nil {
			revert(fmt.Errorf("error creating runtime path: %v", err))
			return
		}
	}

	// Create a link to the downloaded module in the running app path
	codePath := runtimePath + "/" + computationName
	if _, err := os.Stat(codePath); os.IsNotExist(err) {
		err = os.Link(downloadedModulesPath+computationName, codePath)
		if err != nil {
			revert(fmt.Errorf("error linking module: %v", err))
			return
		}
		logger.InfoLogger().Printf("Module linked to running app path: %s", codePath)
	} else {
		logger.InfoLogger().Printf("Module already exists in running app path: %s", codePath)
	}

	//create IPC and memory files
	ipcpath := runtimePath + "/ipc"
	mainmempath := runtimePath + "/main_memory.b"
	checkpointmempath := runtimePath + "/checkpoint_memory.b"
	os.Create(ipcpath)
	os.Create(mainmempath)
	os.Create(checkpointmempath)

	// execute ./create_command comp.wasm ipc_file.txt main_memory.b checkpoint_memory.b
	cmd := exec.Command("/etc/oakestra/wasm/create_command", codePath, ipcpath, mainmempath, checkpointmempath, fmt.Sprintf("%s/%s", NAMESPACE, taskID))
	cmd.Dir = runtimePath
	if err := cmd.Run(); err != nil {
		revert(fmt.Errorf("error executing create command: %v", err))
		return
	}

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
