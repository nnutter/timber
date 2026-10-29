package install

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nnutter/timber/herdr"
)

type Dependencies struct {
	Directory      string
	ConfigFile     string
	KeybindingTOML string
	Link           func(context.Context, string) error
	RenderStatus   func(string) string
}

func New(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install the Herdr plugin and print keybinding instructions",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := herdr.WritePlugin(deps.Directory); err != nil {
				return err
			}
			if err := deps.Link(command.Context(), deps.Directory); err != nil {
				return err
			}
			if _, err := fmt.Fprintln(command.ErrOrStderr(), deps.RenderStatus("installed herdr plugin to "+deps.Directory)); err != nil {
				return err
			}
			if _, err := fmt.Fprintln(command.ErrOrStderr(), deps.RenderStatus("linked herdr plugin nnutter.timber")); err != nil {
				return err
			}
			_, err := fmt.Fprintf(command.OutOrStdout(), "Add this keybinding to %s:\n\n%s", deps.ConfigFile, deps.KeybindingTOML)
			return err
		},
	}
}
