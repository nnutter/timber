package timber

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoFixMovesLegacyRepositoryAndRepairsWorktree(t *testing.T) {
	t.Parallel()
	fixture := getTestRepositoryFixture()
	require.NoError(t, fixture.err)

	home := resolvedTempDir(t)
	runtime := testRuntimeForHome(home, home)
	remotePath := filepath.Join(resolvedTempDir(t), "remote.git")
	require.NoError(t, os.CopyFS(remotePath, os.DirFS(fixture.fixture.remotePath)))

	const repoName = "nnutter/timber"
	legacyPath := filepath.Join(home, ".local", "share", "timber", "repos", "nnutter", "timber.git")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyPath), 0o755))
	require.NoError(t, os.CopyFS(legacyPath, os.DirFS(fixture.fixture.barePath)))
	require.NoError(t, replaceGitRemotePath(legacyPath, fixture.fixture.remotePath, remotePath))

	worktreePath := runtime.managedWorktreePath(repoName, "feature")
	require.NoError(t, os.MkdirAll(filepath.Dir(worktreePath), 0o755))
	runGitCommand(t, legacyPath, "worktree", "add", "-b", "feature", worktreePath, "origin/main")
	internalPath := filepath.Join(legacyPath, "git-review", "worktree")
	runGitCommand(t, legacyPath, "worktree", "add", "--detach", internalPath, "origin/main")

	result := runTimberCommandWithRuntime(t, runtime, "repo", "fix")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "moved repository "+repoName)
	assert.Contains(t, result.stderr, "repaired worktrees for "+repoName)
	assert.DirExists(t, runtime.bareRepoPath(repoName))
	assert.NoDirExists(t, legacyPath)
	assert.NoDirExists(t, filepath.Join(home, ".local", "share", "timber", "repos", "nnutter"))

	content, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	require.NoError(t, err)
	assert.Contains(t, string(content), runtime.bareRepoPath(repoName))
	runGitCommand(t, worktreePath, "status", "--short", "--branch")
	movedInternalPath := filepath.Join(runtime.bareRepoPath(repoName), "git-review", "worktree")
	runGitCommand(t, movedInternalPath, "status", "--short", "--branch")

	again := runTimberCommandWithRuntime(t, runtime, "repo", "fix")
	require.NoError(t, again.err, again.stderr)
	assert.NotContains(t, again.stderr, "moved")
	assert.NotContains(t, again.stderr, "repaired")
}

func TestRepoFixLeavesUnmatchedInternalCheckoutUntouched(t *testing.T) {
	t.Parallel()
	fixture := getTestRepositoryFixture()
	require.NoError(t, fixture.err)
	home := resolvedTempDir(t)
	runtime := testRuntimeForHome(home, home)
	legacyPath := filepath.Join(home, ".local", "share", "timber", "repos", "timber.git")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyPath), 0o755))
	require.NoError(t, os.CopyFS(legacyPath, os.DirFS(fixture.fixture.barePath)))
	oldPath := filepath.Join(legacyPath, "git-review", "worktree")
	runGitCommand(t, legacyPath, "worktree", "add", "--detach", oldPath, "HEAD")
	gitFile := filepath.Join(oldPath, ".git")
	require.NoError(t, os.WriteFile(gitFile, []byte("gitdir: "+filepath.Join(legacyPath, "worktrees", "other")+"\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Dir(runtime.bareRepoPath("timber")), 0o755))
	require.NoError(t, os.Rename(legacyPath, runtime.bareRepoPath("timber")))

	result := runTimberCommandWithRuntime(t, runtime, "repo", "fix")
	require.NoError(t, result.err, result.stderr)
	content, err := os.ReadFile(filepath.Join(runtime.bareRepoPath("timber"), "git-review", "worktree", ".git"))
	require.NoError(t, err)
	assert.Contains(t, string(content), filepath.Join(legacyPath, "worktrees", "other"))
	assert.NotContains(t, result.stderr, "repaired")
}

func TestRepoFixRefusesToOverwriteDestination(t *testing.T) {
	t.Parallel()
	fixture := getTestRepositoryFixture()
	require.NoError(t, fixture.err)

	home := resolvedTempDir(t)
	runtime := testRuntimeForHome(home, home)
	remotePath := filepath.Join(resolvedTempDir(t), "remote.git")
	require.NoError(t, os.CopyFS(remotePath, os.DirFS(fixture.fixture.remotePath)))

	legacyPath := filepath.Join(home, ".local", "share", "timber", "repos", "timber.git")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyPath), 0o755))
	require.NoError(t, os.CopyFS(legacyPath, os.DirFS(fixture.fixture.barePath)))
	require.NoError(t, replaceGitRemotePath(legacyPath, fixture.fixture.remotePath, remotePath))
	require.NoError(t, os.MkdirAll(runtime.bareRepoPath("timber"), 0o755))

	result := runTimberCommandWithRuntime(t, runtime, "repo", "fix")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "already exists")
	assert.DirExists(t, legacyPath)
}

func TestRepoFixRepairsWorktreeAfterBareRepoAlreadyMoved(t *testing.T) {
	t.Parallel()
	fixture := getTestRepositoryFixture()
	require.NoError(t, fixture.err)

	home := resolvedTempDir(t)
	runtime := testRuntimeForHome(home, home)
	remotePath := filepath.Join(resolvedTempDir(t), "remote.git")
	require.NoError(t, os.CopyFS(remotePath, os.DirFS(fixture.fixture.remotePath)))

	legacyPath := filepath.Join(home, ".local", "share", "timber", "repos", "timber.git")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyPath), 0o755))
	require.NoError(t, os.CopyFS(legacyPath, os.DirFS(fixture.fixture.barePath)))
	require.NoError(t, replaceGitRemotePath(legacyPath, fixture.fixture.remotePath, remotePath))

	worktreePath := runtime.managedWorktreePath("timber", "feature")
	require.NoError(t, os.MkdirAll(filepath.Dir(worktreePath), 0o755))
	runGitCommand(t, legacyPath, "worktree", "add", "-b", "feature", worktreePath, "origin/main")
	require.NoError(t, os.MkdirAll(filepath.Dir(runtime.bareRepoPath("timber")), 0o755))
	require.NoError(t, os.Rename(legacyPath, runtime.bareRepoPath("timber")))

	result := runTimberCommandWithRuntime(t, runtime, "repo", "fix")
	require.NoError(t, result.err, result.stderr)
	assert.NotContains(t, result.stderr, "moved")
	assert.Contains(t, result.stderr, "repaired worktrees for timber")
	runGitCommand(t, worktreePath, "status", "--short", "--branch")
}
