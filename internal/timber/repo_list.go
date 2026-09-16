package timber

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/spf13/cobra"
)

type repoListCommandOptions struct {
	runtime  Runtime
	quiet    bool
	sortName bool
}

func NewRepoListCommand(runtime Runtime) *cobra.Command {
	options := &repoListCommandOptions{runtime: runtime}
	command := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List registered repositories",
		Args:    cobra.NoArgs,
		RunE:    options.Execute,
	}
	command.Flags().BoolVarP(&options.quiet, "quiet", "q", false, "Print repository names only")
	command.Flags().BoolVar(&options.sortName, "sort-name", false, "Sort by repository name instead of alias")

	return command
}

func (x *repoListCommandOptions) Execute(command *cobra.Command, args []string) error {
	repos, err := x.runtime.listRegisteredRepos()
	if err != nil {
		return err
	}
	if x.quiet && x.sortName {
		return writeRepoNames(command, repos)
	}

	type repoDetails struct {
		alias  string
		origin string
	}
	details := make(map[string]repoDetails, len(repos))
	for _, repo := range repos {
		origin := repo.originURL(x.runtime)
		details[repo.Name] = repoDetails{alias: repo.alias(x.runtime, origin), origin: origin}
	}
	if !x.sortName {
		slices.SortFunc(repos, func(left, right registeredRepo) int {
			if order := cmp.Compare(details[left.Name].alias, details[right.Name].alias); order != 0 {
				return order
			}
			return cmp.Compare(left.Name, right.Name)
		})
	}
	if x.quiet {
		return writeRepoNames(command, repos)
	}

	tableView := newOutputTable("Name", "Alias", "Origin")
	for _, repo := range repos {
		detail := details[repo.Name]
		tableView.Row(repo.Name, detail.alias, detail.origin)
	}

	_, err = fmt.Fprintln(command.OutOrStdout(), tableView.String())
	return err
}

func writeRepoNames(command *cobra.Command, repos []registeredRepo) error {
	for _, repo := range repos {
		if _, err := fmt.Fprintln(command.OutOrStdout(), repo.Name); err != nil {
			return err
		}
	}
	return nil
}
