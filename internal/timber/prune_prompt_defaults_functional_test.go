package timber

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalPrunePromptDefaultsToCleanMergedWorktrees(t *testing.T) {
	t.Parallel()
	worktrees := []managedWorktree{
		{Repo: "alpha", Name: "merged", Clean: true, Merged: true},
		{Repo: "alpha", Name: "unmerged", Clean: true, Merged: false},
		{Repo: "alpha", Name: "dirty", Clean: false, Merged: true},
	}

	selected, err := (huhWorktreePrompter{}).Prompt(strings.NewReader("\r"), io.Discard, worktrees)

	require.NoError(t, err)
	assert.Equal(t, worktrees[:1], selected)
}
