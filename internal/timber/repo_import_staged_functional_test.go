package timber

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalRepoImportRejectsStagedChanges(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(fixture.clonePath, "README.md"), []byte("staged work\n"), 0o644))
	runGitCommand(t, fixture.clonePath, "add", "README.md")
	require.Equal(t, "M  README.md\n", runGitCommand(t, fixture.clonePath, "status", "--porcelain"))

	result := fixture.importRun(t, "--name", "project")

	require.ErrorContains(t, result.err, "not clean")
	assert.Equal(t, "staged work\n", runGitCommand(t, fixture.clonePath, "show", ":README.md"))
	assert.Equal(t, "M  README.md\n", runGitCommand(t, fixture.clonePath, "status", "--porcelain"))
	assert.NoDirExists(t, fixture.barePath("project"))
	assert.NoDirExists(t, fixture.managedWorktreePath("project", "main"))
	assert.Empty(t, fixture.trashedPaths(t))
}
