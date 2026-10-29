package space

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

type Worktree struct {
	Repo string
	Name string
	Path string
}

type Dependencies struct {
	Resolve      func(io.Reader, string) (Worktree, error)
	Open         func(context.Context, Worktree) error
	Define       func(context.Context, Worktree) error
	Complete     func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
	RenderStatus func(string) string
}

func New(deps Dependencies) *cobra.Command {
	var newSpace bool
	command := &cobra.Command{
		Use:               "space [name[@repo]]",
		Short:             "Set up a Herdr space for a managed Git worktree",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: deps.Complete,
		RunE: func(command *cobra.Command, args []string) error {
			var raw string
			if len(args) == 1 {
				raw = args[0]
			}
			worktree, err := deps.Resolve(command.InOrStdin(), raw)
			if err != nil {
				return err
			}
			message := "defined herdr tabs in current space for "
			if newSpace {
				if err := deps.Open(command.Context(), worktree); err != nil {
					return err
				}
				message = "opened herdr space for "
			} else if err := deps.Define(command.Context(), worktree); err != nil {
				return err
			}
			_, err = fmt.Fprintln(command.ErrOrStderr(), deps.RenderStatus(message+worktree.Name))
			return err
		},
	}
	command.Flags().BoolVarP(&newSpace, "new", "n", false, "Open a new Herdr workspace")
	return command
}
