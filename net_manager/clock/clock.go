// Package clock provides a coarse, 1Hz Unix-seconds clock for the packet
// path. time.Now() costs more than the lookups it would be timestamping, so
// one ticker goroutine updates the value and everyone else just reads it.
package clock

import (
	"sync/atomic"
	"time"
)

var now atomic.Int64

func init() {
	now.Store(time.Now().Unix())
	go func() {
		ticker := time.NewTicker(time.Second)
		for range ticker.C {
			now.Store(time.Now().Unix())
		}
	}()
}

// Unix returns the current time in Unix seconds, accurate to within a second.
func Unix() int64 { return now.Load() }
