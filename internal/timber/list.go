package timber

import (
	"context"
	"encoding/json/v2"
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

// listJSONWorktree is the JSON record for one worktree in `list --json`
// output. It carries the same information as a table row: identity,
// upstream divergence, TODO.md progress, full commit hash, dirtiness,
// merge state, optional commit review progress, and the pull request display
// string when --pr is used. Review is absent when its status cannot be read.
// Worktrees whose git status could not be read report statusError instead
// of divergence data.
type listJSONWorktree struct {
	Name        string          `json:"name"`
	Repo        string          `json:"repo"`
	Path        string          `json:"path"`
	Upstream    string          `json:"upstream"`
	Ahead       int             `json:"ahead"`
	Behind      int             `json:"behind"`
	Commit      string          `json:"commit"`
	Clean       bool            `json:"clean"`
	Merged      bool            `json:"merged"`
	StatusError bool            `json:"statusError"`
	TodoDone    int             `json:"todoDone"`
	TodoTotal   int             `json:"todoTotal"`
	PullRequest string          `json:"pullRequest"`
	Review      *reviewProgress `json:"review,omitzero"`
}

func worktreesToListJSON(worktrees []managedWorktree) []listJSONWorktree {
	records := make([]listJSONWorktree, 0, len(worktrees))
	for _, worktree := range worktrees {
		records = append(records, listJSONWorktree{
			Name:        worktree.Name,
			Repo:        worktree.Repo,
			Path:        worktree.Path,
			Upstream:    worktree.ListStatus.Upstream,
			Ahead:       worktree.ListStatus.Ahead,
			Behind:      worktree.ListStatus.Behind,
			Commit:      worktree.CommitHash,
			Clean:       worktree.Clean,
			Merged:      worktree.Merged,
			StatusError: worktree.ListError,
			TodoDone:    worktree.Todo.Done,
			TodoTotal:   worktree.Todo.Total,
			PullRequest: worktree.PullRequest,
			Review:      worktree.Review,
		})
	}
	return records
}

func reportListJSON(stdout io.Writer, worktrees []managedWorktree) error {
	output, err := json.Marshal(worktreesToListJSON(worktrees))
	if err != nil {
		return fmt.Errorf("encode worktrees as JSON: %w", err)
	}
	_, err = fmt.Fprintln(stdout, string(output))
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
