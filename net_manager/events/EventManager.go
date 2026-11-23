// Package events tracks per-target "last used" activity via a single atomic
// timestamp, cheap enough to touch on every packet. mqtt's interest
// self-destruct timer polls it to decide when a subscription has gone idle.
package events

import (
	"NetManager/clock"
	"sync"
	"sync/atomic"
	"time"
)

// Activity is a per-target last-used timestamp. The zero value is "never touched".
type Activity struct {
	stamp atomic.Int64 // Unix seconds, 0 = never touched
}

// Touch records that target was just used. It runs on the packet path, so it
// reads the shared coarse clock instead of calling time.Now() itself.
//
// The store is skipped once the stamp already matches the current second.
// Activity is shared across every flow of a job, so an unconditional store
// would bounce its cache line between cores on every packet.
func (a *Activity) Touch() {
	now := clock.Unix()
	if a.stamp.Load() != now {
		a.stamp.Store(now)
	}
}

// IdleFor reports how long since the last Touch, or forever if never touched.
func (a *Activity) IdleFor() time.Duration {
	last := a.stamp.Load()
	if last == 0 {
		return time.Duration(1<<63 - 1)
	}
	return time.Since(time.Unix(last, 0))
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]*Activity)
)

// GetOrCreate returns the shared Activity for target, creating it on first use.
func GetOrCreate(target string) *Activity {
	registryMu.RLock()
	a, ok := registry[target]
	registryMu.RUnlock()
	if ok {
		return a
	}

	registryMu.Lock()
	defer registryMu.Unlock()
	if a, ok = registry[target]; ok {
		return a
	}
	a = &Activity{}
	registry[target] = a
	return a
}

// Delete removes the Activity for target, so the registry doesn't grow with every job ever seen.
func Delete(target string) {
	registryMu.Lock()
	delete(registry, target)
	registryMu.Unlock()
}
