package crosvm

import (
	"errors"
	"fmt"
	"github.com/containers/image/v5/docker"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/util/iotools"
	"go_node_engine/util/taskid"
	"go_node_engine/virtualization/internal/allruntimes"
	"go_node_engine/virtualization/internal/crosvm/internal/image"
	"go_node_engine/virtualization/internal/crosvm/internal/instance"
	virtrt "go_node_engine/virtualization/internal/runtime"
	"os/exec"
	"strings"
	"sync"
	"time"
)

func init() {
	allruntimes.Register(string(model.CROSVM_RUNTIME), newRuntime)
}

var ErrNotDeployed = errors.New("specified instance is not deployed")

const executableName = "crosvm"

type Runtime struct {
	error error

	executablePath string
	runtimeDirPath string
	stateDirPath   string
	cacheDirPath   string

	imageStore *image.Store

	lock      sync.RWMutex
	instances map[string]*instance.Instance
}

func newRuntime(info virtrt.RuntimeInfo) virtrt.Runtime {
	executablePath, err := exec.LookPath(executableName)
	if err != nil {
		logger.ErrorLogger().Printf("unable to find crosvm executable (%s): %v\n", executableName, err)
		return &Runtime{
			error: err,
		}
	}

	runtimeDirPath, err := iotools.CreateSubDir(info.RuntimeDirPath, "crosvm", 0o700)
	if err != nil {
		logger.ErrorLogger().Printf("failed to setup runtime directory for crosvm runtime: %v", err)
		return &Runtime{
			error: err,
		}
	}

	stateDirPath, err := iotools.CreateSubDir(info.StateDirPath, "crosvm", 0o700)
	if err != nil {
		logger.ErrorLogger().Printf("failed to setup state directory for crosvm runtime: %v", err)
		return &Runtime{
			error: err,
		}
	}

	cacheDirPath, err := iotools.CreateSubDir(info.CacheDirPath, "crosvm", 0o700)
	if err != nil {
		logger.ErrorLogger().Printf("failed to setup cache directory for crosvm runtime: %v", err)
		return &Runtime{
			error: err,
		}
	}

	imageDirPath, err := iotools.CreateSubDir(cacheDirPath, "images", 0o700)
	if err != nil {
		logger.ErrorLogger().Printf("failed to setup directory for crosvm images: %v", err)
		return &Runtime{
			error: err,
		}
	}

	imageStore, err := image.NewStore(imageDirPath, image.NewContainersSource(docker.Transport))
	if err != nil {
		logger.ErrorLogger().Printf("failed to create crosvm image store: %v", err)
		return &Runtime{
			error: err,
		}
	}

	var infoMsg strings.Builder
	infoMsg.WriteString("created crosvm runtime:\n")
	_, _ = fmt.Fprintf(&infoMsg, "  > crosvm executable: %s\n", executablePath)
	_, _ = fmt.Fprintf(&infoMsg, "  > runtime directory: %s\n", runtimeDirPath)
	logger.InfoLogger().Print(infoMsg.String())

	return &Runtime{
		error: nil,

		executablePath: executablePath,
		runtimeDirPath: runtimeDirPath,
		stateDirPath:   stateDirPath,
		cacheDirPath:   cacheDirPath,

		imageStore: imageStore,

		lock:      sync.RWMutex{},
		instances: make(map[string]*instance.Instance),
	}
}

func (r *Runtime) Deploy(service model.Service, statusChangeNotificationHandler func(service model.Service)) error {
	if r.error != nil {
		return r.error
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	id := taskid.GenerateForModel(&service)
	inst, ok := r.instances[id]
	if !ok {
		var err error
		inst, err = instance.NewInstance(id, service, statusChangeNotificationHandler, r.executablePath, r.runtimeDirPath, r.stateDirPath, r.imageStore)
		if err != nil {
			return err
		}

		r.instances[id] = inst
	}

	if err := inst.Start(); err != nil {
		return err
	}

	return nil
}

func (r *Runtime) Undeploy(sname string, instancenumber int) error {
	if r.error != nil {
		return r.error
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	id := taskid.Generate(sname, instancenumber)
	inst, ok := r.instances[id]
	if !ok {
		return nil
		//return ErrNotDeployed
	}

	if err := inst.Close(); err != nil {
		return err
	}

	delete(r.instances, id)

	return nil
}

func (r *Runtime) Stop() {
	if r.error != nil {
		return
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	var wg sync.WaitGroup
	for id, inst := range r.instances {
		wg.Add(1)

		go func(id string, inst *instance.Instance) {
			defer wg.Done()
			if err := inst.Close(); err != nil {
				logger.ErrorLogger().Printf("unable to stop and close instance %q: %v", id, err)
				// TODO(axiphi): What do we do in case of an error here? It might be problematic to just keep the VM running.
			}
		}(id, inst)
	}

	wg.Wait()
	clear(r.instances)
}

func (r *Runtime) ResourceMonitoring(every time.Duration, notifyHandler func(res []model.Resources)) {
	if r.error != nil {
		return
	}

	ticker := time.NewTicker(every)
	for range ticker.C {
		r.reportResources(notifyHandler)
	}
}

func (r *Runtime) reportResources(notifyHandler func(res []model.Resources)) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	// TODO(axiphi): collect actual resources and parallelize
	var resourcesList []model.Resources = make([]model.Resources, 0)
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
