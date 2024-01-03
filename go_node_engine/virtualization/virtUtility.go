package virtualization

import (
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/model"
	"os"
)

const LOG_SIZE = 1024

// reads the last 100 bytes of the logfile of a container
func getLogs(serviceID string) string {

	file, err := os.Open(fmt.Sprintf("%s/%s", model.GetNodeInfo().LogDirectory, serviceID))
	if err != nil {
		logger.ErrorLogger().Printf("%v", err)
		return ""
	}
	defer file.Close()

	buf := make([]byte, LOG_SIZE)
	stat, err := file.Stat()
	if err != nil {
		logger.ErrorLogger().Printf("%v", err)
		return ""
	}

	var start int64 = 0
	if stat.Size()-LOG_SIZE < 0 {
		start = 0
	} else {
		start = stat.Size() - LOG_SIZE
	}
	n, err := file.ReadAt(buf, start)
	if err != nil {
		return string(buf[:n])
	}
	return string(buf[:n])
}
