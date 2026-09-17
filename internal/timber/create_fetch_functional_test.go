package timber

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalCreateDoesNotCreateWorktreeWhenFetchFails(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	runGitCommand(t, repository.barePath, "remote", "set-url", "origin", filepath.Join(resolvedTempDir(t), "missing.git"))

	result := repository.runTimber(t, "create", at(testRepoName, "feature/offline"))

	assert.Empty(t, result.stdout)
	assert.NoDirExists(t, repository.worktreePath("feature/offline"))
	repository.assertBranchMissing(t, "feature/offline")
	require.ErrorContains(t, result.err, "fetch origin")
}
