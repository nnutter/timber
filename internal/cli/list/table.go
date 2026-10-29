package list

import (
	"fmt"
	"io"
	"strings"

	"github.com/nnutter/timber/internal/display"
)

// WorktreeView contains the data needed to present a worktree in the list table.
type WorktreeView struct {
	Name            string
	Repo            string
	Ahead           int
	Behind          int
	Upstream        string
	DefaultUpstream string
	Merged          bool
	StatusError     bool
	Todo            string
	Review          string
	Commit          string
	Clean           bool
	PullRequest     string
	DisplayPath     string
}

// ReportTable writes the list table to stdout.
func ReportTable(stdout io.Writer, worktrees []WorktreeView, pullRequests, location bool) error {
	headers := []string{"Name", "Repo", "Status", "Todo", "Review", "Commit", "Dirty"}
	if pullRequests {
		headers = append(headers, "PR")
	}
	if location {
		headers = append(headers, "Location")
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

func groupListTableRows(worktrees []WorktreeView, statusFormatter listStatusFormatter, showPullRequests, showLocation bool) [][]string {
	rows := make([][]string, 0, (len(worktrees)+1)/2)
	for index, worktree := range worktrees {
		status := statusFormatter.format(worktree)
		if worktree.StatusError {
			status = "error"
		}
		row := []string{
			worktree.Name,
			worktree.Repo,
			status,
			worktree.Todo,
		}
		row = append(row, worktree.Review, worktree.Commit, formatDirtyStatus(worktree.Clean))
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
