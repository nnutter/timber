package importcmd

import (
	"io"

	"github.com/spf13/cobra"
)

// Dependencies connects the repo import CLI to the application operation.
type Dependencies struct {
	Import func(source, name string, output io.Writer) error
}

// New constructs the command that converts an existing clone into a managed
// Timber repository.
func New(deps Dependencies) *cobra.Command {
	var name string
	command := &cobra.Command{
		Use:   "import <path>",
		Short: "Convert an existing clone into a managed Timber repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return deps.Import(args[0], name, command.ErrOrStderr())
		},
	}
	command.Flags().StringVar(&name, "name", "", "Repository name (default: derived from remote or checkout)")
	return command
}
