package rename

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Dependencies connects the repo rename CLI to the application operation.
type Dependencies struct {
	Rename       func(oldName, newName string) (sourceName, destName string, err error)
	Complete     func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
	RenderStatus func(string) string
}

// New constructs the command that renames a registered repository.
func New(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:     "rename <old-name> <new-name>",
		Aliases: []string{"mv"},
		Short:   "Rename a registered repository and its managed worktrees",
		Args:    cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			sourceName, destName, err := deps.Rename(args[0], args[1])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.ErrOrStderr(), "%s\n", deps.RenderStatus(fmt.Sprintf("renamed repository %s to %s", sourceName, destName)))
			return err
		},
		ValidArgsFunction: deps.Complete,
	}
}
