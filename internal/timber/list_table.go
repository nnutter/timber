package timber

// ListWorktreeView contains the data needed to present a worktree in a table.
type ListWorktreeView struct {
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

func (runtime Runtime) worktreesToListTable(worktrees []managedWorktree, location bool) []ListWorktreeView {
	views := make([]ListWorktreeView, 0, len(worktrees))
	for _, worktree := range worktrees {
		view := ListWorktreeView{
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
	return views
}
