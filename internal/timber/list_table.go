package timber

import (
	"io"

	clilist "github.com/nnutter/timber/internal/cli/list"
)

func (runtime Runtime) reportListTable(stdout io.Writer, worktrees []managedWorktree, pullRequests, location bool) error {
	views := make([]clilist.WorktreeView, 0, len(worktrees))
	for _, worktree := range worktrees {
		view := clilist.WorktreeView{
			Name:            worktree.Name,
			Repo:            worktree.Repo,
			Ahead:           worktree.ListStatus.Ahead,
			Behind:          worktree.ListStatus.Behind,
			Upstream:        worktree.ListStatus.Upstream,
			DefaultUpstream: worktree.DefaultUpstream,
			Merged:          worktree.Merged,
			StatusError:     worktree.ListError,
			Todo:            worktree.Todo.String(),
			Review:          worktree.Review.String(),
			Commit:          worktree.shortCommitHash(),
			Clean:           worktree.Clean,
			PullRequest:     worktree.PullRequest,
		}
		if location {
			view.DisplayPath = runtime.displayHomePath(worktree.Path)
		}
		views = append(views, view)
	}
	return clilist.ReportTable(stdout, views, pullRequests, location)
}
