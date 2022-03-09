package jobs

import (
	"encoding/json"
	"go_node_engine/interfaces"
	"go_node_engine/logger"
	"go_node_engine/model"
	"time"
)

func NodeStatusUpdater(cadence time.Duration) {
	for true {
		select {
		case <-time.After(cadence):
			data, err := json.Marshal(model.GetDynamicInfo())
			if err != nil {
				logger.ErrorLogger().Printf("ERROR: error gathering ndoe info")
				continue
			}
			interfaces.PublishToBroker("information", string(data))
		}
	}
}
