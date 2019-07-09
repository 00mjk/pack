package commands

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
)

func CompletionCommand(logger logging.Logger) *cobra.Command {
	var completionCmd = &cobra.Command{
		Use:   "completion",
		Short: "Outputs completion script location",
		Long: `Generates bash completion script and outputs its location.

To configure your bash shell to load completions for each session, add the following to your '.bashrc' or '.bash_profile':

	. $(pack completion)
	`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			completionPath := filepath.Join(config.PackHome(), "completion")

			if err := cmd.Parent().GenBashCompletionFile(completionPath); err != nil {
				return err
			}

			logger.Info(completionPath)
			return nil
		}),
	}
	return completionCmd
}
