package herdr

import (
	"github.com/spf13/cobra"

	"github.com/nnutter/timber/internal/cli/herdr/install"
	"github.com/nnutter/timber/internal/cli/herdr/space"
)

func New(installDeps install.Dependencies, spaceDeps space.Dependencies) *cobra.Command {
	command := &cobra.Command{Use: "herdr", Short: "Manage the Herdr plugin and spaces"}
	command.AddCommand(install.New(installDeps), space.New(spaceDeps))
	return command
}
