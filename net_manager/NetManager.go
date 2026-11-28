package main

import (
	"NetManager/cmd"
	"NetManager/logger"
)

func main() {
	if err := cmd.Execute(); err != nil {
		logger.ErrorLogger().Printf("NetManager error executing: %v", err)
	}
}
