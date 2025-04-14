package jobs

import (
	"go_node_engine/model"
	"go_node_engine/virtualization"
	"time"
)

// StartServicesMonitoring starts the monitoring of the services
func StartServicesMonitoring(every time.Duration, notifyHandler func(res []model.Resources)) {
	node := model.GetNodeInfo()
	for _, runtime := range node.Technology {
		go virtualization.GetRuntimeMonitoring(runtime).ResourceMonitoring(every, notifyHandler)
	}
}
