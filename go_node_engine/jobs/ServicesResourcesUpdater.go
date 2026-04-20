package jobs

import (
	"go_node_engine/model"
	"go_node_engine/virtualization"
	"time"
)

// StartServicesMonitoring starts the monitoring of the services
func StartServicesMonitoring(
	runtimeManager *virtualization.RuntimeManager,
	every time.Duration,
	notifyHandler func(res []model.Resources),
) {
	node := model.GetNodeInfo()
	for _, runtimeName := range node.Technology {
		runtimeMonitoring := runtimeManager.GetRuntimeMonitoring(runtimeName)
		if runtimeMonitoring != nil {
			go runtimeMonitoring.ResourceMonitoring(every, notifyHandler)
		}
	}
}
