package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

// nolint:unused
var Version = "None"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of NodeEngine",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version)
	},
}
