package timber

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalPruneKeepsWorktreeWithMissingUpstreamRef(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	const branch = "feature/missing-upstream"
	require.NoError(t, repository.runTimber(t, "create", at(testRepoName, branch)).err)
	runGitCommand(t, repository.barePath, "update-ref", "-d", "refs/remotes/origin/main")
	// The upstream configuration remains; only the referenced branch is gone.
	require.Equal(t, "refs/heads/main\n", runGitCommand(t, repository.barePath, "config", "branch."+branch+".merge"))
	head := runGitCommand(t, repository.worktreePath(branch), "rev-parse", "HEAD")

	result := repository.runTimber(t, "prune", at(testRepoName, ""))

	assert.Equal(t, head, runGitCommand(t, repository.worktreePath(branch), "rev-parse", "HEAD"))
	assert.Equal(t, head, runGitCommand(t, repository.barePath, "rev-parse", "refs/heads/"+branch))
	assert.Empty(t, runGitCommand(t, repository.worktreePath(branch), "status", "--porcelain"))
	require.NoError(t, result.err, result.stderr)
}
