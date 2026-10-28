package generate

import (
	"github.com/spf13/cobra"

	"github.com/nnutter/timber/internal/cli/generate/zsh"
)

type generateCommandOptions struct{}

func New(dataHome string) *cobra.Command {
	options := &generateCommandOptions{}

	command := &cobra.Command{
		Use:   `generate`,
		Short: `Generate shell integration wrapping timber`,
		Args:  cobra.NoArgs,
		RunE:  options.Execute,
	}
	command.CompletionOptions.HiddenDefaultCmd = true

	command.AddCommand(zsh.New(dataHome))

	return command
}

func (x *generateCommandOptions) Execute(command *cobra.Command, args []string) error {
	return command.Help()
}
