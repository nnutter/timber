package fix

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Dependencies connects the repo fix CLI to the application operation.
type Dependencies struct {
	Fix          func() ([]string, error)
	RenderStatus func(string) string
}

// New constructs the command that moves bare repositories into the worktree
// root, repairs worktrees, and records the settings Timber expects.
func New(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "fix",
		Short: "Move bare repositories, repair worktrees, and set push.default",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			messages, err := deps.Fix()
			if err != nil {
				return err
			}
			for _, message := range messages {
				if _, err := fmt.Fprintf(command.ErrOrStderr(), "%s\n", deps.RenderStatus(message)); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
