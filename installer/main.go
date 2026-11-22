package main

import (
	"os"

	"github.com/oakestra/oakestra/installer/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
