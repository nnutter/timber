package root

import "github.com/spf13/cobra"

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
	return command
}
