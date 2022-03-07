package jobs

import (
	"encoding/json"
	"go_node_engine/interfaces"
	"go_node_engine/model"
	"log"
	"time"
)

func NodeStatusUpdater(cadence time.Duration) {
	for true {
		select {
		case <-time.After(cadence):
			data, err := json.Marshal(model.GetDynamicInfo())
			if err != nil {
				log.Printf("ERROR: error gathering ndoe info")
				continue
			}
			interfaces.PublishToBroker("information", string(data))
		}
	}
}
