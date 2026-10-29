package importcmd

import (
	"github.com/spf13/cobra"

	"github.com/nnutter/timber/internal/timber"
)

// Dependencies connects the repo import CLI to the application operation.
type Dependencies struct {
	Import        func(source, name string, reporter timber.ImportReporter) error
	HomeDirectory string
	RenderStatus  func(string) string
	RenderWarning func(string) string
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
			return deps.Import(args[0], name, newReporter(command.ErrOrStderr(), deps))
		},
	}
	command.Flags().StringVar(&name, "name", "", "Repository name (default: derived from remote or checkout)")
	return command
}
