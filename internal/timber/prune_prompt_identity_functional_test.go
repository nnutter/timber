package timber

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalPrunePromptDistinguishesSameNameAcrossRepositories(t *testing.T) {
	t.Parallel()
	worktrees := []managedWorktree{
		{Repo: "alpha", Name: "feature", Clean: true},
		{Repo: "beta", Name: "feature", Clean: true},
	}

	// Neither branch is merged. Move down, select beta, and submit.
	selected, err := (huhWorktreePrompter{}).Prompt(strings.NewReader("\x1b[B \r"), io.Discard, worktrees)

	require.NoError(t, err)
	assert.Equal(t, worktrees[1:], selected)
}
