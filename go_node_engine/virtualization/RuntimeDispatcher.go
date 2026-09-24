package virtualization

import (
	"sync"

	"go_node_engine/model"
	"go_node_engine/util/iotools"

	// make sure all runtimes are initialized
	_ "go_node_engine/virtualization/internal/containerd"
	_ "go_node_engine/virtualization/internal/crosvm"
	_ "go_node_engine/virtualization/internal/unikernel"

	virtrt "go_node_engine/virtualization/internal/runtime"
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

	stateDirPath, err := iotools.CreateOakestraStateDir()
	if err != nil {
		return nil, err
	}

	cacheDirPath, err := iotools.CreateOakestraCacheDir()
	if err != nil {
		return nil, err
	}

	info := virtrt.RuntimeInfo{
		RuntimeDirPath: runtimeDirPath,
		StateDirPath:   stateDirPath,
		CacheDirPath:   cacheDirPath,
	}

	onceInitializers := make(map[string]func() virtrt.Runtime)
	for name, initializer := range virtrt.GetInitializers() {
		onceInitializers[name] = sync.OnceValue(func() virtrt.Runtime {
			return initializer(info)
		})
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
