// Package initcmd provides the repository initialization command.
package initcmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Dependencies connects the repo init CLI to the application operation.
type Dependencies struct {
	Init func(name, initialBranch string) (repoName, targetPath string, err error)
	// CompleteArgs completes the repository name argument.
	CompleteArgs func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
	// CompleteBranches completes the --initial-branch value.
	CompleteBranches func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
	RenderStatus     func(string) string
}

// New constructs the command that creates a local bare repository.
func New(deps Dependencies) *cobra.Command {
	var initialBranch string
	command := &cobra.Command{
		Use:   "init <name>",
		Short: "Create a local repository with an empty initial commit",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			repoName, targetPath, err := deps.Init(args[0], initialBranch)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.ErrOrStderr(), "%s\n", deps.RenderStatus("initialized repository "+repoName+" at "+targetPath))
			return err
		},
		ValidArgsFunction: deps.CompleteArgs,
	}
	command.Flags().StringVarP(&initialBranch, "initial-branch", "b", "main", "Initial branch name")
	if err := command.RegisterFlagCompletionFunc("initial-branch", deps.CompleteBranches); err != nil {
		panic(err)
	}
	return command
}
