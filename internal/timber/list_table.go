package timber

// WorktreeStatusView adds repository TODO progress to the current worktree.
type WorktreeStatusView struct {
	Worktree ListWorktreeView
	RepoTodo string
}

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
