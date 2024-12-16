package virtualization

//#cgo CFLAGS: -I/usr/local/lib/wasmtime-go/include
//#cgo LDFLAGS: -L/usr/local/lib/wasmtime-go/ -lwasmtime-go
//#cgo LDFLAGS: -L/usr/local/lib -lwasmtime
// #include <wasi.h>
// #include <wasmtime.h>
// #include <wasm.h>
// #include <doc-wasm.h>
import (
	"errors"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"os"
	"reflect"
	"sync"
	"time"

	C "github.com/bytecodealliance/wasmtime-go/v25"
	"github.com/struCoder/pidusage"
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

func GetWasmRuntime() *WasmRuntime {
	logger.InfoLogger().Print("Getting WASM runtime")
	wasmSingletonOnce.Do(func() {
		wasmRuntime.killQueue = make(map[string]chan bool)
		wasmRuntime.doneQueue = make(map[string]chan bool)
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
	go r.WasmRuntimeCreationRoutine(service, killChannel, doneChannel, startupChannel, errorChannel, statusChangeNotificationHandler)

	success := <-startupChannel
	if !success {
		err := <-errorChannel
		return err
	}

	return nil
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

func (r *WasmRuntime) WasmRuntimeCreationRoutine(
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

	codePath, err := downloadWasmModule(service.Image)
	entry := "_start"

	if err != nil {
		revert(fmt.Errorf("error downloading module: %v", err))
		return
	}

	deploymentChronoStart := time.Now() // START TIME MEASUREMENT

	engcfg := C.NewConfig()
	engcfg.SetEpochInterruption(true)
	engine := C.NewEngineWithConfig(engcfg)
	defer engine.Close()

	code, err := os.ReadFile(codePath)
	if err != nil {
		revert(fmt.Errorf("error reading file %s: %v", codePath, err))
		return
	}

	logPath := fmt.Sprintf("%s/%s", model.GetNodeInfo().LogDirectory, taskID)
	file, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		revert(err)
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.ErrorLogger().Printf("Unable to close log file: %v", err)
		}
	}()

	store := C.NewStore(engine)
	defer store.Close()
	store.SetEpochDeadline(1)

	wasiConfig := C.NewWasiConfig()
	wasiConfig.SetStdoutFile(logPath)
	store.SetWasi(wasiConfig)

	deploymentConfigEnd := time.Since(deploymentChronoStart).Microseconds()

	module, err := C.NewModule(engine, code)
	if err != nil {
		revert(fmt.Errorf("error compiling module: %v", err))
		return
	}
	defer module.Close()
	logger.InfoLogger().Print("Compiled module")
	deploymentCompilationEnd := time.Since(deploymentChronoStart).Microseconds()

	linker := C.NewLinker(engine)
	err = linker.DefineWasi()
	if err != nil {
		revert(fmt.Errorf("error defining WASI: %v", err))
		return
	}
	defer linker.Close()
	deploymentWasiLinkerEnd := time.Since(deploymentChronoStart).Microseconds()

	instance, err := linker.Instantiate(store, module)
	if err != nil {
		revert(fmt.Errorf("error instantiating module: %v", err))
		return
	}
	logger.InfoLogger().Print("Instantiated module")
	deploymentInstantiationEnd := time.Since(deploymentChronoStart).Microseconds()

	run := instance.GetFunc(store, entry)
	if run == nil {
		revert(fmt.Errorf("function %s not found in the module", entry))
		return
	}
	deploymentFuncCallEnd := time.Since(deploymentChronoStart).Microseconds()

	startup <- true

	runResult := make(chan error, 1)
	// Benchmarking strating times
	logger.InfoLogger().Printf("Printing starting times for %s", taskID)
	logger.InfoLogger().Printf("From deployment to coniguration end: %d us", deploymentConfigEnd)
	logger.InfoLogger().Printf("From deployment to compilation: %d us", deploymentCompilationEnd)
	logger.InfoLogger().Printf("From deployment to WASI linking: %d us", deploymentWasiLinkerEnd)
	logger.InfoLogger().Printf("From deployment to instantiation: %d us", deploymentInstantiationEnd)
	logger.InfoLogger().Printf("From deployment to function call: %d us", deploymentFuncCallEnd)

	go func() {
		_, err := run.Call(store)
		runResult <- err
	}()

	select {
	case err := <-runResult:
		if err != nil {
			if exitErr, ok := err.(*C.Error); ok {
				exitCode, _ := exitErr.ExitStatus()
				if exitCode == 0 {
					logger.InfoLogger().Print("Program exited successfully with code 0")
					if service.OneShot {
						service.Status = model.SERVICE_COMPLETED
					} else {
						service.Status = model.SERVICE_DEAD
					}
					statusChangeNotificationHandler(service)
				} else {
					logger.InfoLogger().Printf("Program exited with code %d", exitCode)
					service.Status = model.SERVICE_DEAD
					statusChangeNotificationHandler(service)
				}
			} else {
				// Handle generic errors
				logger.InfoLogger().Printf("Error executing function '%s': %v", entry, err)
				service.Status = model.SERVICE_DEAD
				statusChangeNotificationHandler(service)
			}
		} else {
			logger.InfoLogger().Print("Module execution completed successfully")
			if service.OneShot {
				service.Status = model.SERVICE_COMPLETED
			} else {
				service.Status = model.SERVICE_DEAD
			}
			statusChangeNotificationHandler(service)
		}
	case <-killChannel:
		logger.InfoLogger().Printf("Kill channel message received for WASM module %s", taskID)
		engine.IncrementEpoch()
		err := <-runResult
		if err != nil {
			if exitErr, ok := err.(*C.Trap); ok {
				logger.InfoLogger().Print(exitErr.Message())
				if exitErr.Code() != nil && *exitErr.Code() == C.Interrupt {
					logger.InfoLogger().Print("Module interrupted successfully")
				}
			} else {
				logger.ErrorLogger().Printf("Error after interrupt: %v", err)
			}
		}

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
		select {
		case <-time.After(every):
			resourceList := make([]model.Resources, 0)
			r.channelLock.RLock()
			pid := os.Getpid()
			sysInfo, err := pidusage.GetStat(pid)
			if err != nil {
				logger.ErrorLogger().Printf("Unable to fetch task info: %v", err)
				r.channelLock.RUnlock()
				continue
			}
			for taskid := range r.killQueue {
				resourceList = append(resourceList, model.Resources{
					Cpu:      fmt.Sprintf("%f", sysInfo.CPU),
					Memory:   fmt.Sprintf("%f", sysInfo.Memory),
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
}
