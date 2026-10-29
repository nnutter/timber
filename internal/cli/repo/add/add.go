package add

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Dependencies connects the repo add CLI to the application operation.
type Dependencies struct {
	Add          func(source, name, alias string) (repoName, targetPath string, err error)
	RenderStatus func(string) string
}

// New constructs the command that registers a bare repository.
func New(deps Dependencies) *cobra.Command {
	var name, alias string
	command := &cobra.Command{
		Use:   "add <url-or-path>",
		Short: "Register a bare repository from a remote URL or path",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			repoName, targetPath, err := deps.Add(args[0], name, alias)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.ErrOrStderr(), "%s\n", deps.RenderStatus("added repository "+repoName+" at "+targetPath))
			return err
		},
	}
	command.Flags().StringVar(&name, "name", "", "Repository name (default: derived from URL)")
	command.Flags().StringVar(&alias, "alias", "", "Repository display alias (default: derived from origin)")
	return command
}
