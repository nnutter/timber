package move

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/nnutter/timber/internal/cli/worktreeoutput"
)

// Dependencies connects the move CLI to worktree relocation and process state.
type Dependencies struct {
	Move          func(input io.Reader, raw, destination string) (name, path string, movingCurrentDirectory bool, err error)
	Complete      func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
	RenderStatus  func(string) string
	RenderWarning func(string) string
}

// New constructs the command that moves a managed worktree.
func New(deps Dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:               "move <name[@repo]> <path>",
		Aliases:           []string{"mv"},
		Short:             "Move a managed Git worktree to a new path",
		Args:              cobra.ExactArgs(2),
		RunE:              run(deps),
		ValidArgsFunction: deps.Complete,
	}
	return command
}

func run(deps Dependencies) func(*cobra.Command, []string) error {
	return func(command *cobra.Command, args []string) error {
		name, path, movingCurrentDirectory, err := deps.Move(command.InOrStdin(), args[0], args[1])
		if err != nil {
			return err
		}
		return worktreeoutput.ReportMove(command.ErrOrStderr(), deps.RenderStatus, deps.RenderWarning, name, path, movingCurrentDirectory)
	}
}
