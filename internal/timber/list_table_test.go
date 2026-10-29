package timber

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
