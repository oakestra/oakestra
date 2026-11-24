package virtualization

import (
	"sync"
	"sync/atomic"

	"go_node_engine/model"
	"go_node_engine/util/iotools"

	// make sure all runtimes are initialized
	_ "go_node_engine/virtualization/internal/containerd"
	_ "go_node_engine/virtualization/internal/crosvm"
	_ "go_node_engine/virtualization/internal/unikernel"

	virtrt "go_node_engine/virtualization/internal/runtime"
)

type RuntimeManager struct {
	info    virtrt.RuntimeInfo
	entries map[string]*runtimeEntry
}

type runtimeEntry struct {
	once  sync.Once
	ready atomic.Bool
	value virtrt.Runtime
	init  func([]string) virtrt.Runtime
}

func (e *runtimeEntry) get(config []string) virtrt.Runtime {
	e.once.Do(func() {
		e.value = e.init(config)
		e.ready.Store(true)
	})
	return e.value
}

func (e *runtimeEntry) peek() virtrt.Runtime {
	if !e.ready.Load() {
		return nil
	}
	return e.value
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

	entries := make(map[string]*runtimeEntry)
	for name, initializer := range virtrt.GetInitializers() {
		init := initializer

		entries[name] = &runtimeEntry{
			init: func(config []string) virtrt.Runtime {
				return init(info, config)
			},
		}
	}

	return &RuntimeManager{
		info:    info,
		entries: entries,
	}, nil
}

func (m *RuntimeManager) PeekRuntime(
	runtime model.RuntimeType,
) virtrt.RuntimeInterface {
	return m.entries[string(runtime)].peek()
}

func (m *RuntimeManager) GetRuntime(
	runtime model.RuntimeType,
	config []string,
) virtrt.RuntimeInterface {
	return m.entries[string(runtime)].get(config)
}

func (m *RuntimeManager) GetRuntimeMonitoring(
	runtime model.RuntimeType,
	config []string,
) virtrt.RuntimeMonitoring {
	return m.entries[string(runtime)].get(config)
}
