package remove

import (
	"fmt"

	"github.com/spf13/cobra"
)

// New constructs the command that removes a registered repository.
func New(remove func(string) error, complete func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective), renderStatus func(string) string) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   "Remove a registered repository with no worktrees",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			name := args[0]
			if err := remove(name); err != nil {
				return err
			}
			_, err := fmt.Fprintln(command.ErrOrStderr(), renderStatus("removed repository "+name))
			return err
		},
		ValidArgsFunction: complete,
	}
}
