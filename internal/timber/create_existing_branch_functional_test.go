package timber

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalCreateChecksOutExistingLocalCommit(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	local := filepath.Join(resolvedTempDir(t), "local")
	runGitCommand(t, repository.barePath, "worktree", "add", "-b", "feature/existing", local, "main")
	require.NoError(t, os.WriteFile(filepath.Join(local, "local.txt"), []byte("local-only\n"), 0o644))
	runGitCommand(t, local, "add", "local.txt")
	runGitCommand(t, local, "commit", "-m", "local-only change")
	head := runGitCommand(t, local, "rev-parse", "HEAD")
	require.NotEqual(t, head, runGitCommand(t, repository.barePath, "rev-parse", "origin/main"))
	runGitCommand(t, repository.barePath, "worktree", "remove", local)

	result := repository.runTimber(t, "create", at(testRepoName, "feature/existing"))

	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, head, runGitCommand(t, repository.worktreePath("feature/existing"), "rev-parse", "HEAD"))
	assert.Equal(t, "feature/existing\n", runGitCommand(t, repository.worktreePath("feature/existing"), "branch", "--show-current"))
}
