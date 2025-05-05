package crosvm

import (
	"errors"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/util/dirutil"
	"go_node_engine/util/taskid"
	cvinstance "go_node_engine/virtualization/crosvm/internal/instance"
	"google.golang.org/genproto/googleapis/spanner/admin/instance/v1"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var ErrNotDeployed = errors.New("specified instance is not deployed")

const executableName = "crosvm"

type Runtime struct {
	executablePath string
	runtimeDirPath string
	errors         []error

	lock      sync.RWMutex
	instances map[string]*cvinstance.Instance
}

var runtimeSingleton *Runtime = nil
var newRuntimeOnce = sync.Once{}

func RuntimeSingleton(baseRuntimeDirPath string) *Runtime {
	newRuntimeOnce.Do(func() {
		runtimeSingleton = newRuntime(baseRuntimeDirPath)
	})
	return runtimeSingleton
}

func newRuntime(baseRuntimeDirPath string) *Runtime {
	var creationErrors []error

	executablePath, err := exec.LookPath(executableName)
	if err != nil {
		creationErrors = append(creationErrors, err)
		logger.ErrorLogger().Printf("Unable to find crosvm executable (%s): %v\n", executableName, err)
	}

	runtimeDirPath, err := dirutil.CreateSubDir(baseRuntimeDirPath, "runtime-crosvm", 0o700)
	if err != nil {
		creationErrors = append(creationErrors, err)
		logger.ErrorLogger().Printf("Failed to setup runtime directory for crosvm runtime: %v\n", err)
	}

	if len(creationErrors) == 0 {
		var infoMsg strings.Builder
		infoMsg.WriteString("Enabled crosvm runtime:\n")
		_, _ = fmt.Fprintf(&infoMsg, "  > crosvm executable: %s\n", executablePath)
		_, _ = fmt.Fprintf(&infoMsg, "  > runtime directory: %s\n", runtimeDirPath)

		logger.InfoLogger().Print(infoMsg)
		model.GetNodeInfo().AddSupportedTechnology(model.CROSVM_RUNTIME)
	}

	return &Runtime{
		executablePath: executablePath,
		runtimeDirPath: runtimeDirPath,
		errors:         creationErrors,

		instances: make(map[string]*instance.Instance),
	}
}

func (r *Runtime) Deploy(service model.Service, statusChangeNotificationHandler func(service model.Service)) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	id := taskid.GenerateForModel(&service)
	instance, ok := r.instances[id]
	if !ok {
		instance, err := cvinstance.NewInstance(id, service, statusChangeNotificationHandler, r.executablePath, r.runtimeDirPath)
		if err != nil {
			return err
		}

		r.instances[id] = instance
	}

	if err := instance.Start(); err != nil {
		return err
	}

	return nil
}

func (r *Runtime) Undeploy(sname string, instancenumber int) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	id := taskid.Generate(sname, instancenumber)
	instance, ok := r.instances[id]
	if !ok {
		return ErrNotDeployed
	}

	if err := instance.Close(); err != nil {
		return err
	}

	delete(r.instances, id)

	return nil
}

func (r *Runtime) Stop() {
	r.lock.Lock()
	defer r.lock.Unlock()

	var wg sync.WaitGroup
	for id, instance := range r.instances {
		wg.Add(1)

		go func(id string, instance *cvinstance.Instance) {
			defer wg.Done()
			if err := instance.Close(); err != nil {
				logger.ErrorLogger().Printf("rt-crosvm: Unable to stop and close instance %q: %v", id, err)
				// TODO(axiphi): What do we do in case of an error here? It might be problematic to just keep the VM running.
			}
		}(id, instance)
	}

	wg.Wait()
	clear(r.instances)
}

func (r *Runtime) ResourceMonitoring(every time.Duration, notifyHandler func(res []model.Resources)) {
	ticker := time.NewTicker(every)
	for range ticker.C {
		r.reportResources(notifyHandler)
	}
}

func (r *Runtime) reportResources(notifyHandler func(res []model.Resources)) {
	r.lock.RLock()
	defer r.lock.Unlock()

	// TODO(axiphi): collect actual resources and parallelize
	var resourcesList []model.Resources
	for id, _ := range r.instances {
		serviceName := taskid.ExtractServiceName(id)
		instanceNumber := taskid.ExtractInstanceNumber(id)
		resourcesList = append(resourcesList, model.Resources{
			Cpu:      "0.0",
			Memory:   "0.0",
			Disk:     "0",
			Sname:    serviceName,
			Logs:     "",
			Runtime:  string(model.CROSVM_RUNTIME),
			Instance: instanceNumber,
		})
	}
	notifyHandler(resourcesList)
}
