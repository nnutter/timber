package timber

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type listCommandOptions struct {
	repoSelection
	pullRequests bool
}

func NewListCommand(runtime Runtime) *cobra.Command {
	options := &listCommandOptions{runtime: runtime}

	command := &cobra.Command{
		Use:               "list [@repo]",
		Aliases:           []string{"ls"},
		Short:             "List managed Git worktrees",
		Args:              cobra.MaximumNArgs(1),
		RunE:              options.Execute,
		ValidArgsFunction: runtime.completeRepoQualifiers,
	}
	command.Flags().BoolVar(&options.pullRequests, "pr", false, "Include open pull request status from gh")
	return command
}

func (x *listCommandOptions) Execute(command *cobra.Command, args []string) error {
	if err := x.applyRepoArg(args); err != nil {
		return err
	}

	worktrees, err := x.collectWorktrees()
	if err != nil {
		return err
	}
	if x.pullRequests {
		if err := x.runtime.enrichListedWorktreesWithPullRequests(command.Context(), worktrees); err != nil {
			return err
		}
	}

	headers := []string{"Name", "Repo", "Status", "Commit", "Dirty", "Merged"}
	if x.pullRequests {
		headers = append(headers, "PR")
	}
	statusFormatter := newListStatusFormatter(worktrees)
	tableView := newOutputTable(headers...).BorderRow(true)
	tableView.Rows(groupListTableRows(worktrees, statusFormatter, x.pullRequests)...)

	_, err = fmt.Fprintln(command.OutOrStdout(), dottedListRowRules(tableView.String()))
	return err
}

func dottedListRowRules(tableOutput string) string {
	lines := strings.Split(tableOutput, "\n")
	headerRuleFound := false
	for index, line := range lines {
		if !strings.HasPrefix(line, "├") {
			continue
		}
		if !headerRuleFound {
			headerRuleFound = true
			continue
		}
		lines[index] = strings.ReplaceAll(line, "─", "┈")
	}
	return strings.Join(lines, "\n")
}

func groupListTableRows(worktrees []managedWorktree, statusFormatter listStatusFormatter, showPullRequests bool) [][]string {
	rows := make([][]string, 0, (len(worktrees)+1)/2)
	for index, worktree := range worktrees {
		status := statusFormatter.format(worktree.ListStatus)
		if worktree.ListError {
			status = "error"
		}
		row := []string{
			worktree.Name,
			worktree.Repo,
			status,
			worktree.shortCommitHash(),
			formatDirtyStatus(worktree.Clean),
			strconv.FormatBool(worktree.Merged),
		}
		if showPullRequests {
			row = append(row, worktree.PullRequest)
		}
		if index%2 == 0 {
			rows = append(rows, row)
			continue
		}
		for column := range row {
			rows[len(rows)-1][column] += "\n" + row[column]
		}
	}
	return rows
}

func formatDirtyStatus(clean bool) string {
	dirty := strconv.FormatBool(!clean)
	if !clean {
		return warningStyle.Render(dirty)
	}
	return dirty
}

func (x *listCommandOptions) collectWorktrees() ([]managedWorktree, error) {
	repos, err := x.reposToConsider()
	if err != nil {
		return nil, err
	}
	return x.runtime.collectListedWorktrees(repos)
}

func (x *listCommandOptions) applyRepoArg(args []string) error {
	if len(args) == 0 {
		return nil
	}
	repo, err := x.runtime.parseRepoOnlyArg(args[0])
	if err != nil {
		return err
	}
	x.RepoName = repo
	return nil
}
