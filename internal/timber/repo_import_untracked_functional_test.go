package timber

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalRepoImportRejectsUntrackedFiles(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	path := filepath.Join(fixture.clonePath, "scratch.txt")
	require.NoError(t, os.WriteFile(path, []byte("untracked work\n"), 0o644))
	// A user's status preferences must not hide files from the preflight.
	runGitCommand(t, fixture.clonePath, "config", "status.showUntrackedFiles", "no")

	result := fixture.importRun(t, "--name", "project")

	require.ErrorContains(t, result.err, "not clean")
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "untracked work\n", string(contents))
	assert.NoDirExists(t, fixture.barePath("project"))
	assert.NoDirExists(t, fixture.managedWorktreePath("project", "main"))
	assert.Empty(t, fixture.trashedPaths(t))
}
