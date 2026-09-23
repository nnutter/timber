package timber

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListStatusFormatterAlignsAndColorsIndicators(t *testing.T) {
	t.Parallel()
	worktrees := []managedWorktree{
		{ListStatus: listStatus{Upstream: "origin/dev", Ahead: 18, Behind: 62}},
		{ListStatus: listStatus{Upstream: "origin/dev", Ahead: 1, Behind: 391}},
		{ListStatus: listStatus{Upstream: "origin/dev", Behind: 161}},
		{ListStatus: listStatus{Upstream: "origin/dev"}},
		{},
	}

	formatter := newListStatusFormatter(worktrees)

	assert.Equal(
		t,
		aheadStatusStyle.Render("↑18")+" "+behindStatusStyle.Render("↓ 62")+" [origin/dev]",
		formatter.format(worktrees[0]),
	)
	assert.Equal(
		t,
		aheadStatusStyle.Render("↑ 1")+" "+behindStatusStyle.Render("↓391")+" [origin/dev]",
		formatter.format(worktrees[1]),
	)
	assert.Equal(
		t,
		"    "+behindStatusStyle.Render("↓161")+" [origin/dev]",
		formatter.format(worktrees[2]),
	)
	assert.Equal(t, "         [origin/dev]", formatter.format(worktrees[3]))
	assert.Empty(t, formatter.format(worktrees[4]))
}

func TestListStatusFormatterShowsMergedInsteadOfDivergence(t *testing.T) {
	t.Parallel()
	worktrees := []managedWorktree{
		{
			Merged:          true,
			ListStatus:      listStatus{Upstream: "origin/main", Ahead: 3, Behind: 1},
			DefaultUpstream: "origin/main",
		},
		{
			Merged:          true,
			ListStatus:      listStatus{Upstream: "origin/dev", Ahead: 3},
			DefaultUpstream: "origin/main",
		},
	}

	formatter := newListStatusFormatter(worktrees)

	assert.Equal(t, "merged", formatter.format(worktrees[0]))
	assert.Equal(t, "merged [origin/dev]", formatter.format(worktrees[1]))
}

func TestListStatusFormatterHidesDefaultUpstream(t *testing.T) {
	t.Parallel()
	worktrees := []managedWorktree{
		{ListStatus: listStatus{Upstream: "origin/main", Ahead: 2}, DefaultUpstream: "origin/main"},
		{ListStatus: listStatus{Upstream: "origin/dev", Ahead: 2}, DefaultUpstream: "origin/main"},
	}

	formatter := newListStatusFormatter(worktrees)

	assert.Equal(t, aheadStatusStyle.Render("↑2"), formatter.format(worktrees[0]))
	assert.Equal(
		t,
		aheadStatusStyle.Render("↑2")+" [origin/dev]",
		formatter.format(worktrees[1]),
	)
}
