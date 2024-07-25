package main

import (
	"go_node_engine/cmd"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		fmt.Println(os.Stderr, "NodeEngine error executing: %v\n", err)
	}
}
