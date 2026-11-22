package cli

import "github.com/spf13/cobra"

// Execute runs the installer command line.
func Execute() error {
	command := &cobra.Command{
		Use:   "installer",
		Short: "Install Oakestra components on this machine",
	}
	showHelp := func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	}
	command.AddCommand(
		&cobra.Command{
			Use:   "root",
			Short: "Install the Oakestra root orchestrator",
			Args:  cobra.NoArgs,
			RunE:  showHelp,
		},
		&cobra.Command{
			Use:   "cluster",
			Short: "Install an Oakestra cluster orchestrator",
			Args:  cobra.NoArgs,
			RunE:  showHelp,
		},
		&cobra.Command{
			Use:   "worker",
			Short: "Install an Oakestra worker node",
			Args:  cobra.NoArgs,
			RunE:  showHelp,
		},
	)
	return command.Execute()
}
