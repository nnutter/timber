package timber

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalRepoImportChecksLinkedWorktreesBeforeMutation(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	linked := filepath.Join(filepath.Dir(fixture.clonePath), "linked")
	runGitCommand(t, fixture.clonePath, "worktree", "add", "-b", "feature", linked)
	require.NoError(t, os.WriteFile(filepath.Join(linked, "README.md"), []byte("linked work\n"), 0o644))
	require.Empty(t, runGitCommand(t, fixture.clonePath, "status", "--porcelain"))

	result := fixture.importRun(t, "--name", "project")

	require.ErrorContains(t, result.err, "not clean")
	assert.Contains(t, result.err.Error(), linked)
	fixture.assertWorktreeBranchAt(t, fixture.clonePath, "main")
	fixture.assertWorktreeBranchAt(t, linked, "feature")
	contents, err := os.ReadFile(filepath.Join(linked, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "linked work\n", string(contents))
	assert.NoDirExists(t, fixture.barePath("project"))
	assert.NoDirExists(t, fixture.managedWorktreePath("project", "main"))
	assert.NoDirExists(t, fixture.managedWorktreePath("project", "feature"))
	assert.Empty(t, fixture.trashedPaths(t))
}
