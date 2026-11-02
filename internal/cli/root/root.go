package root

import (
	"github.com/spf13/cobra"

	"github.com/nnutter/timber/internal/cli/complete"
)

func New(homeDirectory, skillContent string, commands ...*cobra.Command) *cobra.Command {
	options := &skillCommandOptions{homeDirectory: homeDirectory, content: skillContent}
	command := &cobra.Command{
		Use:           "timber",
		Short:         "Manage Git worktrees",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE:          options.Execute,
	}
	command.Flags().BoolVar(&options.install, "install-skill", false, "Install the timber skill to ~/.agents/skills")
	command.Flags().BoolVarP(&options.force, "force", "f", false, "Overwrite an existing installed skill")
	command.CompletionOptions.HiddenDefaultCmd = true
	command.AddCommand(commands...)
	setAliasExpansions(command)
	return command
}

// setAliasExpansions lets completion expand a short alias into the name of
// the command it stands for. It applies to this command and to every command
// group below it, so an alias stays a single definition in one command.
func setAliasExpansions(command *cobra.Command) {
	if command.HasSubCommands() {
		if command.ValidArgsFunction == nil {
			command.ValidArgsFunction = complete.AliasExpansions()
		}
	}
	for _, subcommand := range command.Commands() {
		setAliasExpansions(subcommand)
	}
}
