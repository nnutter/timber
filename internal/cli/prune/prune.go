package prune

import (
	"io"

	"github.com/spf13/cobra"
)

// Dependencies connects the prune CLI to worktree pruning.
type Dependencies struct {
	Prune    func(input io.Reader, output io.Writer, repoArg string, prompt, dryRun bool) error
	Complete func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
}

// New constructs the command that prunes managed worktrees.
func New(deps Dependencies) *cobra.Command {
	var prompt, dryRun bool
	command := &cobra.Command{
		Use:               "prune [@repo]",
		Aliases:           []string{"clean"},
		Short:             "Prune managed Git worktrees",
		Args:              cobra.MaximumNArgs(1),
		RunE:              run(deps, &prompt, &dryRun),
		ValidArgsFunction: deps.Complete,
	}
	command.Flags().BoolVarP(&prompt, "prompt", "p", false, "Prompt before pruning")
	command.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "List worktrees that would be pruned")
	return command
}

func run(deps Dependencies, prompt, dryRun *bool) func(*cobra.Command, []string) error {
	return func(command *cobra.Command, args []string) error {
		var repoArg string
		if len(args) == 1 {
			repoArg = args[0]
		}
		return deps.Prune(command.InOrStdin(), command.ErrOrStderr(), repoArg, *prompt, *dryRun)
	}
}
