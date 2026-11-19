package timber

import (
	"io"
)

// ListOptions controls how ListWorktrees filters, sorts, and renders worktrees.
type ListOptions struct {
	RepoArg       string
	PullRequests  bool
	Todo          bool
	Review        bool
	Location      bool
	LocationsOnly bool
	JSON          bool
	SortMode      string
}

// ListRenderers provides the output functions used by ListWorktrees.
type ListRenderers struct {
	Table     func(io.Writer, []ListWorktreeView, bool, bool, bool, bool) error
	JSON      func(io.Writer, []ListJSONWorktree) error
	Locations func(io.Writer, []string) error
}
