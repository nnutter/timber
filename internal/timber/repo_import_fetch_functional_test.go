package timber

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalRepoImportCanRetryAfterOriginFetchFails(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	runGitCommand(t, fixture.clonePath, "remote", "set-url", "origin", filepath.Join(resolvedTempDir(t), "missing.git"))
	head := runGitCommand(t, fixture.clonePath, "rev-parse", "HEAD")

	result := fixture.importRun(t, "--name", "project")

	require.ErrorContains(t, result.err, "fetch")
	assert.Equal(t, head, runGitCommand(t, fixture.clonePath, "rev-parse", "HEAD"))
	assert.Empty(t, runGitCommand(t, fixture.clonePath, "status", "--porcelain"))
	assert.NotContains(t, fixture.trashedPaths(t), fixture.clonePath)
	assert.NoDirExists(t, fixture.barePath("project"))
	assert.NoDirExists(t, fixture.managedWorktreePath("project", "main"))

	runGitCommand(t, fixture.clonePath, "remote", "set-url", "origin", fixture.remotePath)
	retry := fixture.importRun(t, "--name", "project")
	require.NoError(t, retry.err, retry.stderr)
	fixture.assertWorktreeBranchAt(t, fixture.managedWorktreePath("project", "main"), "main")
}
