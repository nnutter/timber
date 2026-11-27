package create

import (
	"context"
	"io"

	"github.com/spf13/cobra"
)

// Dependencies connects the create CLI to worktree creation and process state.
type Dependencies struct {
	Create     func(ctx context.Context, input io.Reader, output io.Writer, repoName, raw, explicitPath string, hasExplicitPath bool, upstream string, herdr, noHerdr bool) (path string, herdrOpened bool, err error)
	ReportPath func(output io.Writer, path string) error
	Complete   func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
}

// New constructs the command that creates a managed worktree.
func New(deps Dependencies) *cobra.Command {
	var upstream string
	var herdr, noHerdr bool
	command := &cobra.Command{
		Use:               "create [name[@repo]] [path]",
		Short:             "Create a managed Git worktree",
		Args:              cobra.MaximumNArgs(2),
		RunE:              run(deps, &upstream, &herdr, &noHerdr),
		ValidArgsFunction: deps.Complete,
	}
	command.Flags().StringVarP(&upstream, "upstream", "u", "", "Upstream branch")
	command.Flags().BoolVar(&herdr, "herdr", false, "Also create a Herdr workspace for the new worktree")
	command.Flags().BoolVar(&noHerdr, "no-herdr", false, "Do not create a Herdr workspace")
	command.MarkFlagsMutuallyExclusive("herdr", "no-herdr")
	return command
}

func run(deps Dependencies, upstream *string, herdr, noHerdr *bool) func(*cobra.Command, []string) error {
	return func(command *cobra.Command, args []string) error {
		var raw, explicitPath string
		var hasExplicitPath bool
		if len(args) > 0 {
			raw = args[0]
		}
		if len(args) == 2 {
			explicitPath = args[1]
			hasExplicitPath = true
		}
		worktreePath, _, err := deps.Create(
			command.Context(),
			command.InOrStdin(),
			command.ErrOrStderr(),
			"",
			raw,
			explicitPath,
			hasExplicitPath,
			*upstream,
			*herdr,
			*noHerdr,
		)
		if err != nil {
			return err
		}
		return deps.ReportPath(command.OutOrStdout(), worktreePath)
	}
}
