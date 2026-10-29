package tui

import (
	"context"
	"io"

	"github.com/spf13/cobra"
)

// Dependencies connects the TUI CLI to the interactive wizard.
type Dependencies struct {
	Run      func(ctx context.Context, input io.Reader, output, stdout io.Writer, herdr, noHerdr, noTitle bool) error
	Complete func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
}

// New constructs the command that interactively creates or opens a worktree.
func New(deps Dependencies) *cobra.Command {
	var herdr, noHerdr, noTitle bool
	command := &cobra.Command{
		Use:   "tui",
		Short: "Interactively create a worktree or open an existing one",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return deps.Run(command.Context(), command.InOrStdin(), command.ErrOrStderr(), command.OutOrStdout(), herdr, noHerdr, noTitle)
		},
	}
	command.Flags().BoolVar(&herdr, "herdr", false, "Also create a Herdr workspace for a new worktree")
	command.Flags().BoolVar(&noHerdr, "no-herdr", false, "Do not create a Herdr workspace")
	command.Flags().BoolVar(&noTitle, "no-title", false, "Hide the title header")
	command.MarkFlagsMutuallyExclusive("herdr", "no-herdr")
	return command
}
