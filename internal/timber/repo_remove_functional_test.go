package timber

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalRepoRemoveRefusesUnmanagedLinkedWorktrees(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	unmanaged := filepath.Join(resolvedTempDir(t), "unmanaged")
	runGitCommand(t, repository.barePath, "worktree", "add", unmanaged, "main")
	head := runGitCommand(t, unmanaged, "rev-parse", "HEAD")

	result := repository.runTimber(t, "repo", "remove", testRepoName)

	require.DirExists(t, repository.barePath)
	assert.Equal(t, head, runGitCommand(t, unmanaged, "rev-parse", "HEAD"))
	assert.Equal(t, head, runGitCommand(t, repository.barePath, "rev-parse", "refs/heads/main"))
	assert.Empty(t, runGitCommand(t, unmanaged, "status", "--porcelain"))
	require.ErrorContains(t, result.err, "linked worktree")
}
