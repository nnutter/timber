package timber

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalRepoImportRejectsModifiedWorktree(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	path := filepath.Join(fixture.clonePath, "README.md")
	require.NoError(t, os.WriteFile(path, []byte("unfinished work\n"), 0o644))

	result := fixture.importRun(t, "--name", "project")

	require.ErrorContains(t, result.err, "not clean")
	assert.Contains(t, result.err.Error(), fixture.clonePath)
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "unfinished work\n", string(contents))
	assert.NoDirExists(t, fixture.barePath("project"))
	assert.NoDirExists(t, fixture.managedWorktreePath("project", "main"))
	assert.Empty(t, fixture.trashedPaths(t))
}
