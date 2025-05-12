package virtualization

import (
	"go_node_engine/model"
	"go_node_engine/util/iotools"
	"go_node_engine/virtualization/internal/allruntimes"
	virtrt "go_node_engine/virtualization/internal/runtime"
	"sync"
)

type RuntimeManager struct {
	info         virtrt.RuntimeInfo
	initializers map[string]func() virtrt.Runtime
}

func NewRuntimeManager() (*RuntimeManager, error) {
	runtimeDirPath, err := iotools.CreateOakestraRuntimeDir()
	if err != nil {
		return nil, err
	}

	cacheDirPath, err := iotools.CreateOakestraCacheDir()
	if err != nil {
		return nil, err
	}

	info := virtrt.RuntimeInfo{
		RuntimeDirPath: runtimeDirPath,
		CacheDirPath:   cacheDirPath,
	}

	var onceInitializers map[string]func() virtrt.Runtime
	for name, initializer := range allruntimes.GetInitializers() {
		onceInitializers[name] = sync.OnceValue(func() virtrt.Runtime {
			return initializer(info)
		})
		// maybe this shouldn't be global state
		model.GetNodeInfo().AddSupportedTechnology(model.RuntimeType(name))
	}

	return &RuntimeManager{
		info:         info,
		initializers: onceInitializers,
	}, nil
}

func (m *RuntimeManager) GetRuntime(runtime model.RuntimeType) virtrt.RuntimeInterface {
	return m.initializers[string(runtime)]()
}

func (m *RuntimeManager) GetRuntimeMonitoring(runtime model.RuntimeType) virtrt.RuntimeMonitoring {
	return m.initializers[string(runtime)]()
}
