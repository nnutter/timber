package switchcmd

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// Dependencies connects the switch CLI to worktree lookup, creation, and
// per-worktree hook configuration.
type Dependencies struct {
	Resolve        func(input io.Reader, raw, repoName string) (name, path string, alreadyIn bool, err error)
	Create         func(ctx context.Context, input io.Reader, output io.Writer, repoName, raw, explicitPath string, hasExplicitPath bool, upstream string, herdr, noHerdr bool) (path string, herdrOpened bool, err error)
	ConfigureHooks func(path string) error
	ReportPath     func(output io.Writer, path string) error
	Complete       func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
}

// New constructs the command that resolves a managed worktree path.
func New(deps Dependencies) *cobra.Command {
	var create, noCd bool
	var upstream string
	var herdr, noHerdr bool
	command := &cobra.Command{
		Use:               "switch [name[@repo]]",
		Aliases:           []string{"sw"},
		Short:             "Resolve a managed worktree path",
		Args:              cobra.ExactArgs(1),
		RunE:              run(deps, &create, &noCd, &upstream, &herdr, &noHerdr),
		Hidden:            true,
		ValidArgsFunction: deps.Complete,
	}
	command.Flags().BoolVarP(&create, "create", "c", false, "Create the worktree if it does not exist")
	command.Flags().BoolVar(&noCd, "no-cd", false, "Create without reporting a path to change to")
	command.Flags().StringVarP(&upstream, "upstream", "u", "", "Upstream branch")
	command.Flags().BoolVar(&herdr, "herdr", false, "Also create a Herdr workspace for the new worktree")
	command.Flags().BoolVar(&noHerdr, "no-herdr", false, "Do not create a Herdr workspace")
	command.MarkFlagsMutuallyExclusive("herdr", "no-herdr")
	return command
}

func run(deps Dependencies, create, noCd *bool, upstream *string, herdr, noHerdr *bool) func(*cobra.Command, []string) error {
	return func(command *cobra.Command, args []string) error {
		if *create {
			worktreePath, herdrOpened, err := deps.Create(
				command.Context(),
				command.InOrStdin(),
				command.ErrOrStderr(),
				"",
				args[0],
				"",
				false,
				*upstream,
				*herdr,
				*noHerdr,
			)
			if err != nil {
				return err
			}
			if *noCd || herdrOpened {
				return nil
			}
			return deps.ReportPath(command.OutOrStdout(), worktreePath)
		}
		name, worktreePath, alreadyIn, err := deps.Resolve(command.InOrStdin(), args[0], "")
		if err != nil {
			return err
		}
		if err := deps.ConfigureHooks(worktreePath); err != nil {
			return err
		}
		if alreadyIn {
			if _, err := fmt.Fprintf(command.ErrOrStderr(), "Already in %s\n", name); err != nil {
				return err
			}
		}
		return deps.ReportPath(command.OutOrStdout(), worktreePath)
	}
}
