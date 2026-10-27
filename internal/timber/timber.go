package timber

import "github.com/spf13/cobra"

func NewRootCommand(runtime Runtime) *cobra.Command {
	options := &skillCommandOptions{runtime: runtime}
	rootCommand := &cobra.Command{
		Use:           "timber",
		Short:         "Manage Git worktrees",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE:          options.Execute,
	}
	rootCommand.Flags().BoolVar(&options.install, "install-skill", false, "Install the timber skill to ~/.agents/skills")
	rootCommand.Flags().BoolVarP(&options.force, "force", "f", false, "Overwrite an existing installed skill")
	rootCommand.CompletionOptions.HiddenDefaultCmd = true

	rootCommand.AddCommand(NewConfigCommand(runtime))
	rootCommand.AddCommand(NewCreateCommand(runtime))
	rootCommand.AddCommand(NewListCommand(runtime))
	rootCommand.AddCommand(NewPruneCommand(runtime))
	rootCommand.AddCommand(NewRemoveCommand(runtime))
	rootCommand.AddCommand(NewRepoCommand(runtime))
	rootCommand.AddCommand(NewReviewCommand(runtime))
	rootCommand.AddCommand(NewHerdrCommand(runtime))
	rootCommand.AddCommand(NewSwitchCommand(runtime))
	rootCommand.AddCommand(NewTodoCommand(runtime))
	rootCommand.AddCommand(NewGitCommand(runtime))
	rootCommand.AddCommand(NewTUICommand(runtime))
	rootCommand.AddCommand(NewGenerateCommand(runtime))

	return rootCommand
}
