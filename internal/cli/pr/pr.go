// Package pr assembles the GitHub pull request commands.
package pr

import "github.com/spf13/cobra"

// New assembles the pull request command tree.
func New(commands ...*cobra.Command) *cobra.Command {
	command := &cobra.Command{
		Use:   "pr",
		Short: "Work with GitHub pull requests",
	}
	command.AddCommand(commands...)
	return command
}
