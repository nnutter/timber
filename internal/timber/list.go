package timber

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/nnutter/timber/internal/display"
)

// ListWorktrees lists managed worktrees as a table or JSON document. All
// output is written to the given writers, preserving CLI reporting order.
func (runtime Runtime) ListWorktrees(ctx context.Context, stdout io.Writer, repoArg string, pullRequests, location, jsonOutput bool, sortMode string) error {
	var sortBy listSort
	if err := sortBy.Set(sortMode); err != nil {
		return err
	}

	selection := repoSelection{runtime: runtime}
	if repoArg != "" {
		repo, err := runtime.parseRepoOnlyArg(repoArg)
		if err != nil {
			return err
		}
		selection.RepoName = repo
	}

	repos, err := selection.reposToConsider()
	if err != nil {
		return err
	}
	enrich := func(repository *Repository, worktree managedWorktree) (managedWorktree, error) {
		return enrichWorktreeForSort(runtime, repository, worktree, sortBy)
	}
	worktrees, err := runtime.collectWorktrees(repos, enrich)
	if err != nil {
		return err
	}
	sortBy.sort(worktrees)
	runtime.enrichListedWorktreesWithReviews(ctx, worktrees)
	if pullRequests {
		if err := runtime.enrichListedWorktreesWithPullRequests(ctx, worktrees); err != nil {
			return err
		}
	}

	if jsonOutput {
		return reportListJSON(stdout, worktrees)
	}
	return runtime.reportListTable(stdout, worktrees, pullRequests, location)
}

func (runtime Runtime) reportListTable(stdout io.Writer, worktrees []managedWorktree, pullRequests, location bool) error {
	headers := []string{"Name", "Repo", "Status", "Todo", "Review", "Commit", "Dirty"}
	if pullRequests {
		headers = append(headers, "PR")
	}
	if location {
		headers = append(headers, "Location")
		for index := range worktrees {
			worktrees[index].DisplayPath = runtime.displayHomePath(worktrees[index].Path)
		}
	}
	statusFormatter := newListStatusFormatter(worktrees)
	tableView := display.NewOutputTable(headers...).BorderRow(true)
	tableView.Rows(groupListTableRows(worktrees, statusFormatter, pullRequests, location)...)

	_, err := fmt.Fprintln(stdout, dottedListRowRules(tableView.String()))
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

func groupListTableRows(worktrees []managedWorktree, statusFormatter listStatusFormatter, showPullRequests, showLocation bool) [][]string {
	rows := make([][]string, 0, (len(worktrees)+1)/2)
	for index, worktree := range worktrees {
		status := statusFormatter.format(worktree)
		if worktree.ListError {
			status = "error"
		}
		row := []string{
			worktree.Name,
			worktree.Repo,
			status,
			worktree.Todo.String(),
		}
		row = append(row, worktree.Review.String(), worktree.shortCommitHash(), formatDirtyStatus(worktree.Clean))
		if showPullRequests {
			row = append(row, worktree.PullRequest)
		}
		if showLocation {
			row = append(row, worktree.DisplayPath)
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
	if !clean {
		return display.WarningStyle.Render("dirty")
	}
	return "clean"
}
