package jobs

import (
	"encoding/json"
	"go_node_engine/logger"
	"go_node_engine/model"
	"go_node_engine/mqtt"
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
			mqtt.PublishToBroker("information", string(data))
		}
	}
}
