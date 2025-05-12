package allruntimes

import (
	virtrt "go_node_engine/virtualization/internal/runtime"
	"maps"
	"sync"
)

type RuntimeInitializer func(info virtrt.RuntimeInfo) virtrt.Runtime

var runtimeInitializers map[string]RuntimeInitializer
var lock sync.RWMutex

func Register(name string, initializer RuntimeInitializer) {
	lock.Lock()
	defer lock.Unlock()
	runtimeInitializers[name] = initializer
}

func GetInitializers() map[string]RuntimeInitializer {
	lock.RLock()
	defer lock.RUnlock()
	return maps.Clone(runtimeInitializers)
}
