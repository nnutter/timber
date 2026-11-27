// Package hooks assembles the Git hook commands.
package hooks

import (
	"github.com/spf13/cobra"

	"github.com/nnutter/timber/internal/cli/hooks/commitmsg"
)

// New builds the hooks command tree for one checkout.
func New(directory string) *cobra.Command {
	command := &cobra.Command{
		Use:   "hooks",
		Short: "Run Git hooks that Timber provides",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(commitmsg.New(directory))
	return command
}
