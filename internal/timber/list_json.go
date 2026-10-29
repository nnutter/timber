package timber

import (
	"io"

	clilist "github.com/nnutter/timber/internal/cli/list"
)

func reportListJSON(stdout io.Writer, worktrees []managedWorktree) error {
	records := make([]clilist.JSONWorktree, 0, len(worktrees))
	for _, worktree := range worktrees {
		record := clilist.JSONWorktree{
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
		}
		if worktree.Review != nil {
			record.Review = &clilist.JSONReviewProgress{
				Approved: worktree.Review.Approved,
				Total:    worktree.Review.Total,
			}
		}
		records = append(records, record)
	}
	return clilist.ReportJSON(stdout, records)
}
