package runtime

import (
	"maps"
	"sync"
)

type RuntimeInitializer func(info RuntimeInfo) Runtime

var runtimeInitializers = make(map[string]RuntimeInitializer)
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
