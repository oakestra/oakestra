package crosvm

import (
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/util"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const executableName = "crosvm"

type Runtime struct {
	executablePath string
	runtimeDirPath string
	errors         []error

	instances map[string]*instance
}

type CrosvmAction int

const (
	CrosvmActionUndeploy CrosvmAction = iota
)

type CrosvmInstance struct {
	actionQueue chan CrosvmAction
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
	var errors []error

	executablePath, err := exec.LookPath(executableName)
	if err != nil {
		errors = append(errors, err)
		logger.ErrorLogger().Printf("Unable to find crosvm executable (%s): %v\n", executableName, err)
	}

	runtimeDirPath, err := util.CreateSubRuntimeDir(baseRuntimeDirPath, "runtime-crosvm")
	if err != nil {
		errors = append(errors, err)
		logger.ErrorLogger().Printf("Failed to setup runtime directory for crosvm runtime: %v\n", err)
	}

	if len(errors) == 0 {
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
		errors:         errors,
	}
}

func (r *Runtime) Deploy(service model.Service, statusChangeNotificationHandler func(service model.Service)) error {
	//instance, err := newInstance(r, &service)
	//if err != nil {
	//	return err
	//}

	//go instance.run(r)
	return nil
}

func (r *Runtime) Undeploy(sname string, instance int) error {
	return nil
}

func (r *Runtime) Stop() {

}

func (r *Runtime) ResourceMonitoring(every time.Duration, notifyHandler func(res []model.Resources)) {

}
