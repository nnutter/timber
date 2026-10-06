package timber

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomWorktreeRemovalPreservesParents(t *testing.T) {
	t.Parallel()
	for _, command := range []string{"remove", "prune"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			repo := newTestRepository(t)
			// Empty parents inside HOME would previously be removed along
			// with the checkout. Two conflicts also exercise prune's snapshot.
			for _, group := range []string{"one", "two"} {
				path := filepath.Join(repo.home, "projects", group, "checkout")
				runGitCommand(t, repo.barePath, "worktree", "add", "-b", "feature/"+group, path, "origin/main")
			}
			args := []string{command}
			if command == "remove" {
				args = append(args, "one/checkout")
			}
			result := repo.runTimber(t, args...)
			require.NoError(t, result.err, result.stderr)
			assert.NoDirExists(t, filepath.Join(repo.home, "projects", "one", "checkout"))
			assert.DirExists(t, filepath.Join(repo.home, "projects", "one"))
			if command == "prune" {
				assert.NoDirExists(t, filepath.Join(repo.home, "projects", "two", "checkout"))
				assert.DirExists(t, filepath.Join(repo.home, "projects", "two"))
			}
		})
	}
}

func TestManualWorktreeWithoutUpstreamRequiresForceRemoval(t *testing.T) {
	t.Parallel()
	repo := newTestRepository(t)
	path := filepath.Join(repo.home, "projects", "checkout")
	runGitCommand(t, repo.barePath, "worktree", "add", "-b", "manual-no-upstream", path, "main")
	runGitCommandAllowError(t, repo.barePath, "branch", "--unset-upstream", "manual-no-upstream")
	pruned := repo.runTimber(t, "prune")
	require.NoError(t, pruned.err, pruned.stderr)
	assert.DirExists(t, path)
	rejected := repo.runTimber(t, "remove", "checkout")
	require.Error(t, rejected.err)
	assert.Contains(t, rejected.err.Error(), "has no upstream branch")
	result := repo.runTimber(t, "remove", "--force", "checkout")
	require.NoError(t, result.err, result.stderr)
	assert.NoDirExists(t, path)
	assert.DirExists(t, filepath.Dir(path))
}
