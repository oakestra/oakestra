package cli

import (
	"context"
	"log/slog"

	charmlog "charm.land/log/v2"
	"github.com/oakestra/oakestra/installer/cluster"
	"github.com/oakestra/oakestra/installer/root"
	"github.com/oakestra/oakestra/installer/worker"
	"github.com/spf13/cobra"
)

// Execute runs the installer command line.
func Execute(ctx context.Context) error {
	command := &cobra.Command{
		Use:   "installer",
		Short: "Install Oakestra components on this machine",
	}
	var autoConfirm bool
	command.PersistentFlags().BoolVarP(&autoConfirm, "yes", "y", false, "Skip confirmation prompts")

	logger := slog.New(charmlog.New(command.OutOrStdout()))
	newInstallCommand := func(name, description string, install func(*slog.Logger) error) *cobra.Command {
		return &cobra.Command{
			Use:   name,
			Short: description,
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				cmd.SilenceErrors = true
				cmd.SilenceUsage = true
				return install(logger)
			},
		}
	}
	var clusterAddr string
	workerCommand := &cobra.Command{
		Use:   "worker [version]",
		Short: "Install an Oakestra worker node",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true

			var version string
			if len(args) == 1 {
				version = args[0]
			}
			err := worker.Install(cmd.Context(), logger, version, clusterAddr, autoConfirm)
			if err != nil {
				logger.Error(err.Error())
			}
			return err
		},
	}
	workerCommand.Flags().StringVar(&clusterAddr, "cluster-addr", "", "Cluster Orchestrator host or HTTP(S) URL (required with --yes)")
	command.AddCommand(
		newInstallCommand("root", "Install the Oakestra root orchestrator", root.Install),
		newInstallCommand("cluster", "Install an Oakestra cluster orchestrator", cluster.Install),
		workerCommand,
	)
	return command.ExecuteContext(ctx)
}
