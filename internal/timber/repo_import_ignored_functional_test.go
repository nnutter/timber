package timber

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalRepoImportRejectsIgnoredFiles(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(fixture.clonePath, ".gitignore"), []byte(".env\n"), 0o644))
	runGitCommand(t, fixture.clonePath, "add", ".gitignore")
	runGitCommand(t, fixture.clonePath, "commit", "-m", "ignore local settings")
	path := filepath.Join(fixture.clonePath, ".env")
	require.NoError(t, os.WriteFile(path, []byte("LOCAL_SETTING=keep\n"), 0o600))
	require.Empty(t, runGitCommand(t, fixture.clonePath, "status", "--porcelain"))

	result := fixture.importRun(t, "--name", "project")

	require.ErrorContains(t, result.err, "not clean")
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "LOCAL_SETTING=keep\n", string(contents))
	assert.NoDirExists(t, fixture.barePath("project"))
	assert.NoDirExists(t, fixture.managedWorktreePath("project", "main"))
	assert.Empty(t, fixture.trashedPaths(t))
}
