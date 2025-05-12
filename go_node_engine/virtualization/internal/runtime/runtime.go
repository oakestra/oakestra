package runtime

import (
	"go_node_engine/model"
	"time"
)

type RuntimeInfo struct {
	RuntimeDirPath string
	CacheDirPath   string
}

type Runtime interface {
	RuntimeInterface
	RuntimeMonitoring
}

type RuntimeInterface interface {
	Deploy(service model.Service, statusChangeNotificationHandler func(service model.Service)) error
	Undeploy(sname string, instance int) error
	Stop()
}

type RuntimeMonitoring interface {
	ResourceMonitoring(every time.Duration, notifyHandler func(res []model.Resources))
}
