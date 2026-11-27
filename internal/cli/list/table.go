package list

import (
	"fmt"
	"io"
	"strings"

	"github.com/nnutter/timber/internal/display"
	"github.com/nnutter/timber/internal/timber"
)

// ReportTable writes the list table to stdout.
func ReportTable(stdout io.Writer, worktrees []timber.ListWorktreeView, pullRequests, todo, review, location bool) error {
	headers := []string{"Name", "Repo", "Status"}
	if todo {
		headers = append(headers, "Todo")
	}
	if review {
		headers = append(headers, "Review")
	}
	headers = append(headers, "Commit", "Dirty")
	if pullRequests {
		headers = append(headers, "PR")
	}
	if location {
		headers = append(headers, "Location")
	}
	statusFormatter := newListStatusFormatter(worktrees)
	rows := groupListTableRows(worktrees, statusFormatter, pullRequests, todo, review, location)
	return reportTable(stdout, headers, rows)
}

func reportTable(stdout io.Writer, headers []string, rows [][]string) error {
	tableView := display.NewOutputTable(headers...).BorderRow(true)
	tableView.Rows(rows...)

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

func groupListTableRows(worktrees []timber.ListWorktreeView, statusFormatter listStatusFormatter, showPullRequests, showTodo, showReview, showLocation bool) [][]string {
	rows := make([][]string, 0, (len(worktrees)+1)/2)
	for index, worktree := range worktrees {
		row := []string{worktree.Name, worktree.Repo, statusFormatter.format(worktree)}
		if showTodo {
			row = append(row, worktree.Todo)
		}
		if showReview {
			row = append(row, worktree.Review)
		}
		row = append(row, worktree.Commit, formatDirtyStatus(worktree.Clean))
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
