package timber

import (
	"encoding/json/v2"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type listCommandOptions struct {
	repoSelection
	pullRequests bool
	jsonOutput   bool
	sortBy       listSort
}

func NewListCommand(runtime Runtime) *cobra.Command {
	options := &listCommandOptions{runtime: runtime, sortBy: listSortRepo}

	command := &cobra.Command{
		Use:               "list [@repo]",
		Aliases:           []string{"ls"},
		Short:             "List managed Git worktrees",
		Args:              cobra.MaximumNArgs(1),
		RunE:              options.Execute,
		ValidArgsFunction: runtime.completeRepoQualifiers,
	}
	command.Flags().BoolVar(&options.pullRequests, "pr", false, "Include open pull request status from gh")
	command.Flags().BoolVar(&options.jsonOutput, "json", false, "Output worktrees as JSON instead of a table")
	command.Flags().Var(&options.sortBy, "sort", "Sort worktrees by repo or worktree")
	if err := command.RegisterFlagCompletionFunc("sort", completeListSort); err != nil {
		panic(err)
	}
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
	x.sortBy.sort(worktrees)
	if x.pullRequests {
		if err := x.runtime.enrichListedWorktreesWithPullRequests(command.Context(), worktrees); err != nil {
			return err
		}
	}

	if x.jsonOutput {
		return x.reportJSON(command, worktrees)
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

// listJSONWorktree is the JSON record for one worktree in `list --json`
// output. It carries the same information as a table row: identity,
// upstream divergence, full commit hash, dirtiness, merge state, and the
// pull request display string when --pr is used. Worktrees whose git status
// could not be read report statusError instead of divergence data.
type listJSONWorktree struct {
	Name        string `json:"name"`
	Repo        string `json:"repo"`
	Path        string `json:"path"`
	Upstream    string `json:"upstream"`
	Ahead       int    `json:"ahead"`
	Behind      int    `json:"behind"`
	Commit      string `json:"commit"`
	Clean       bool   `json:"clean"`
	Merged      bool   `json:"merged"`
	StatusError bool   `json:"statusError"`
	PullRequest string `json:"pullRequest"`
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
			PullRequest: worktree.PullRequest,
		})
	}
	return records
}

func (x *listCommandOptions) reportJSON(command *cobra.Command, worktrees []managedWorktree) error {
	output, err := json.Marshal(worktreesToListJSON(worktrees))
	if err != nil {
		return fmt.Errorf("encode worktrees as JSON: %w", err)
	}
	_, err = fmt.Fprintln(command.OutOrStdout(), string(output))
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
