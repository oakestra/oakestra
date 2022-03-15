package jobs

import (
	"go_node_engine/model"
	"sync"
	"time"
)

var once sync.Once

func NodeStatusUpdater(cadence time.Duration, statusUpdateHandler func(node model.Node)) {
	once.Do(func() {
		go updateRoutine(cadence, statusUpdateHandler)
	})
}

func updateRoutine(cadence time.Duration, statusUpdateHandler func(node model.Node)) {
	for true {
		select {
		case <-time.After(cadence):
			statusUpdateHandler(model.GetDynamicInfo())
		}
	}
}
