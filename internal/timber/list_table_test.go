package timber

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/display"
)

func TestGroupListTableRowsAddsRuleAfterEverySecondWorktree(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		worktrees          []managedWorktree
		groupedNames       []string
		horizontalRuleRows int
	}{
		{
			name: "odd number of worktrees",
			worktrees: []managedWorktree{
				{Name: "one", Clean: true},
				{Name: "two", Clean: true},
				{Name: "three", Clean: true},
				{Name: "four", Clean: true},
				{Name: "five", Clean: true},
			},
			groupedNames:       []string{"one\ntwo", "three\nfour", "five"},
			horizontalRuleRows: 3,
		},
		{
			name: "even number of worktrees",
			worktrees: []managedWorktree{
				{Name: "one", Clean: true},
				{Name: "two", Clean: true},
				{Name: "three", Clean: true},
				{Name: "four", Clean: true},
			},
			groupedNames:       []string{"one\ntwo", "three\nfour"},
			horizontalRuleRows: 2,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			rows := groupListTableRows(testCase.worktrees, newListStatusFormatter(testCase.worktrees), false, false)
			require.Len(t, rows, len(testCase.groupedNames))
			for index, names := range testCase.groupedNames {
				assert.Equal(t, names, rows[index][0])
			}

			tableView := display.NewOutputTable("Name", "Repo", "Status", "Todo", "Review", "Commit", "Dirty").BorderRow(true)
			tableView.Rows(rows...)
			tableOutput := dottedListRowRules(tableView.String())
			assert.Equal(t, testCase.horizontalRuleRows, strings.Count(tableOutput, "├"))
			assert.Equal(t, 1, strings.Count(tableOutput, "├─"))
			assert.Equal(t, testCase.horizontalRuleRows-1, strings.Count(tableOutput, "├┈"))
		})
	}
}

func TestGroupListTableRowsIncludesPullRequestColumnWhenEnabled(t *testing.T) {
	t.Parallel()
	worktrees := []managedWorktree{
		{Name: "one", Clean: true, PullRequest: "#42 ✓"},
		{Name: "two", Clean: true},
	}

	rows := groupListTableRows(worktrees, newListStatusFormatter(worktrees), true, false)
	require.Len(t, rows, 1)
	assert.Equal(t, "#42 ✓\n", rows[0][7])

	rows = groupListTableRows(worktrees, newListStatusFormatter(worktrees), false, false)
	require.Len(t, rows, 1)
	assert.Len(t, rows[0], 7)
}

func TestFormatDirtyStatusColorsTrueYellow(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "clean", formatDirtyStatus(true))
	assert.Equal(t, display.WarningStyle.Render("dirty"), formatDirtyStatus(false))
}

func TestListRecencyOrdering(t *testing.T) {
	t.Parallel()
	worktrees := []managedWorktree{
		{Repo: "b", Name: "a", CommitTime: time.Unix(100, 0)},
		{Repo: "a", Name: "z", CommitTime: time.Unix(100, 0)},
		{Repo: "a", Name: "a", CommitTime: time.Unix(100, 0)},
		{Name: "unborn"},
		{Name: "newest", CommitTime: time.Unix(200, 0)},
		{Name: "oldest", CommitTime: time.Unix(-100, 0)},
	}
	listSortRecency.sort(worktrees)
	var names []string
	for _, worktree := range worktrees {
		names = append(names, worktree.Name+"@"+worktree.Repo)
	}
	assert.Equal(t, []string{"newest@", "a@a", "z@a", "a@b", "oldest@", "unborn@"}, names)
}
