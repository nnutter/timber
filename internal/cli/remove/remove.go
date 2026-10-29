package remove

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// Dependencies connects the remove CLI to worktree removal and process state.
type Dependencies struct {
	Remove        func(io.Reader, string, bool) (name, shortHash, path string, removingCurrentDirectory bool, err error)
	Complete      func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
	RenderStatus  func(string) string
	RenderWarning func(string) string
}

// New constructs the command that removes a managed worktree.
func New(deps Dependencies) *cobra.Command {
	var force bool
	command := &cobra.Command{
		Use:               "remove [-f|--force] [name[@repo]]",
		Aliases:           []string{"rm"},
		Short:             "Remove a managed Git worktree",
		Args:              cobra.MaximumNArgs(1),
		RunE:              run(deps, &force),
		ValidArgsFunction: deps.Complete,
	}
	command.Flags().BoolVarP(&force, "force", "f", false, "Force removal")
	return command
}

func run(deps Dependencies, force *bool) func(*cobra.Command, []string) error {
	return func(command *cobra.Command, args []string) error {
		var raw string
		if len(args) == 1 {
			raw = args[0]
		}
		name, shortHash, path, removingCurrentDirectory, err := deps.Remove(command.InOrStdin(), raw, *force)
		if err != nil {
			return err
		}
		return reportRemoval(command.ErrOrStderr(), deps, name, shortHash, path, removingCurrentDirectory)
	}
}

func reportRemoval(output io.Writer, deps Dependencies, name, shortHash, path string, removingCurrentDirectory bool) error {
	if _, err := os.Stat(path); err == nil {
		if _, writeErr := fmt.Fprintf(output, "%s\n", deps.RenderWarning("warning: worktree directory still exists: "+path)); writeErr != nil {
			return writeErr
		}
	}
	if _, err := fmt.Fprintf(output, "%s\n", deps.RenderStatus(fmt.Sprintf("removed %s at %s", name, shortHash))); err != nil {
		return err
	}
	if removingCurrentDirectory {
		_, err := fmt.Fprintf(output, "%s\n", deps.RenderWarning("current directory was removed"))
		return err
	}
	return nil
}
