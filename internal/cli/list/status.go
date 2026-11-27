package list

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"github.com/nnutter/timber/internal/timber"
)

// NewStatus constructs the status command for the current worktree.
func NewStatus(query func(context.Context, io.Reader) (timber.WorktreeStatusView, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the current worktree's status",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			status, err := query(command.Context(), command.InOrStdin())
			if err != nil {
				return err
			}
			return reportStatusTable(command.OutOrStdout(), status)
		},
	}
}

func reportStatusTable(output io.Writer, status timber.WorktreeStatusView) error {
	worktree := status.Worktree
	formatter := newListStatusFormatter([]timber.ListWorktreeView{worktree})
	headers := []string{"Status", "TODO", "Repo TODO", "Review", "Commit", "Dirty", "PR"}
	row := []string{
		formatter.format(worktree), worktree.Todo, status.RepoTodo, worktree.Review,
		worktree.Commit, formatDirtyStatus(worktree.Clean), worktree.PullRequest,
	}
	return reportTable(output, headers, [][]string{row})
}
