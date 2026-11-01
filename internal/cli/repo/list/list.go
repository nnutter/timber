// Package list provides the repository list command.
package list

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/nnutter/timber/internal/display"
	"github.com/nnutter/timber/internal/timber/repo"
)

// New constructs the command that lists registered repositories.
func New(query func(bool) ([]repo.Repo, error)) *cobra.Command {
	var quiet, sortName bool
	command := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List registered repositories",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			repos, err := query(!quiet || isFalse(sortName))
			if err != nil {
				return err
			}
			if !sortName {
				slices.SortFunc(repos, func(left, right repo.Repo) int {
					if order := cmp.Compare(left.Alias, right.Alias); order != 0 {
						return order
					}
					return cmp.Compare(left.Name, right.Name)
				})
			}
			if quiet {
				for _, repo := range repos {
					if _, err := fmt.Fprintln(command.OutOrStdout(), repo.Name); err != nil {
						return err
					}
				}
				return nil
			}
			tableView := display.NewOutputTable("Name", "Alias", "Origin")
			for _, repo := range repos {
				tableView.Row(repo.Name, repo.Alias, repo.Origin)
			}
			_, err = fmt.Fprintln(command.OutOrStdout(), tableView.String())
			return err
		},
	}
	command.Flags().BoolVarP(&quiet, "quiet", "q", false, "Print repository names only")
	command.Flags().BoolVar(&sortName, "sort-name", false, "Sort by repository name instead of alias")
	return command
}
