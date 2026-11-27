package repo

import "github.com/spf13/cobra"

// New assembles the repository command tree.
func New(commands ...*cobra.Command) *cobra.Command {
	command := &cobra.Command{
		Use:   "repo",
		Short: "Manage registered bare repositories",
	}
	command.AddCommand(commands...)
	return command
}
