package main

import (
	"go_node_engine/cmd"
	"go_node_engine/logger"

)

func main() {
	if err := cmd.Execute(); err != nil {
		logger.ErrorLogger().Printf("NodeEngine error executing: %v", err)
	}
}
