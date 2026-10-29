package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveRemovesEmptyParentDirectories(t *testing.T) {
	t.Parallel()
	const branchName = "feature/nested/path"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.mergeWorktreeBranch(t, branchName)

	result := testRepository.runTimber(t, "remove", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)

	testRepository.assertPathMissing(t, filepath.Join(testRepository.worktreeRoot, testRepoName, "feature"))
}

func TestRemoveFailsWhenDirtyWithoutForce(t *testing.T) {
	t.Parallel()
	const branchName = "feature/dirty"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.writeFileInWorktree(t, branchName, "dirty.txt", "dirty\n")

	result := testRepository.runTimber(t, "remove", at(testRepoName, branchName))
	contents, err := os.ReadFile(filepath.Join(testRepository.worktreePath(branchName), "dirty.txt"))
	require.NoError(t, err)
	assert.Equal(t, "dirty\n", string(contents))
	runGitCommand(t, testRepository.barePath, "show-ref", "--verify", "refs/heads/"+branchName)
	require.ErrorContains(t, result.err, "not clean")
}

func TestRemoveWithNoArgsRemovesCurrentWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/current"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.mergeWorktreeBranch(t, branchName)

	result := testRepository.runTimberFrom(t, testRepository.worktreePath(branchName), "remove")
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}

func TestRemoveWithNoArgsFromSubdirectoryRemovesCurrentWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/subdir"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.mergeWorktreeBranch(t, branchName)

	subDir := filepath.Join(testRepository.worktreePath(branchName), "nested")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	result := testRepository.runTimberFrom(t, subDir, "remove")
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}

func TestRemoveMainWhenInSyncWithUpstream(t *testing.T) {
	t.Parallel()

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "main")).err)

	result := testRepository.runTimber(t, "remove", at(testRepoName, "main"))
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathMissing(t, testRepository.worktreePath("main"))
}

func TestRemoveFailsWhenUnmergedWithoutForce(t *testing.T) {
	t.Parallel()
	const branchName = "feature/unmerged"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.commitFileInWorktree(t, branchName, "extra.txt", "extra\n")
	head := runGitCommand(t, testRepository.worktreePath(branchName), "rev-parse", "HEAD")

	result := testRepository.runTimber(t, "remove", at(testRepoName, branchName))
	assert.Equal(t, head, runGitCommand(t, testRepository.worktreePath(branchName), "rev-parse", "HEAD"))
	assert.Equal(t, head, runGitCommand(t, testRepository.barePath, "rev-parse", "refs/heads/"+branchName))
	require.ErrorContains(t, result.err, "not merged")
}

func TestRemoveFetchesOriginBeforeCheckingMerged(t *testing.T) {
	t.Parallel()
	const branchName = "feature/stale-merged"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.commitFileInWorktree(t, branchName, "extra.txt", "extra\n")

	// Publish the branch so a separate clone can merge it, then merge
	// and push main there. The managed bare repo never fetches in
	// between, so its origin/main stays stale until remove fetches.
	runGitCommand(t, testRepository.worktreePath(branchName), "push", "origin", branchName)
	updaterPath := filepath.Join(resolvedTempDir(t), "updater")
	runGitCommand(t, filepath.Dir(updaterPath), "clone", testRepository.remotePath, updaterPath)
	runGitCommand(t, updaterPath, "config", "user.name", "Test User")
	runGitCommand(t, updaterPath, "config", "user.email", "test@example.com")
	runGitCommand(t, updaterPath, "fetch", "origin")
	runGitCommand(t, updaterPath, "checkout", "main")
	runGitCommand(t, updaterPath, "merge", "--ff-only", "origin/"+branchName)
	runGitCommand(t, updaterPath, "push", "origin", "main")

	result := testRepository.runTimber(t, "remove", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
	testRepository.assertBranchMissing(t, branchName)
}

func TestRemoveForceRemovesDirtyUnmergedWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/force"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.commitFileInWorktree(t, branchName, "extra.txt", "extra\n")
	testRepository.writeFileInWorktree(t, branchName, "dirty.txt", "dirty\n")

	result := testRepository.runTimber(t, "remove", "--force", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
	testRepository.assertBranchMissing(t, branchName)
}

func TestRemoveCompletionOffersManagedWorktreeNames(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/a")).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/b")).err)

	stdout := runCompleteWithRuntime(t, testRepository.runtime, "remove", "")
	assert.Contains(t, stdout, "feature/a")
	assert.Contains(t, stdout, "feature/b")
}

func TestRemoveCompletionUsesCurrentWorktreeRepoWhenRepoFlagOmitted(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/a")).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/b")).err)

	runtime := testRepository.runtime
	runtime.CurrentDirectory = testRepository.worktreePath("feature/a")
	stdout := runCompleteWithRuntime(t, runtime, "remove", "")
	assert.Contains(t, stdout, "feature/a")
	assert.Contains(t, stdout, "feature/b")
}

func TestRemoveCompletionOutsideManagedWorktreeOffersUniqueNames(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/a")).err)

	stdout := runCompleteWithRuntime(t, testRepository.runtime, "remove", "")
	assert.Contains(t, stdout, "feature/a")
	assert.NotContains(t, stdout, "feature/a@")
}

func TestRemovePreservesReferenceLikeBranchNames(t *testing.T) {
	t.Parallel()
	const branchName = "refs-like/name"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.mergeWorktreeBranch(t, branchName)

	result := testRepository.runTimber(t, "remove", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	testRepository.assertBranchMissing(t, branchName)
}

func TestRemoveInfersUniqueRepoOutsideWorktree(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	registerAdditionalRepo(t, primary, "other")
	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/login")).err)
	primary.mergeWorktreeBranch(t, "feature/login")

	result := primary.runTimberFrom(t, primary.home, "remove", "feature/login")
	require.NoError(t, result.err, result.stderr)
	primary.assertPathMissing(t, primary.worktreePath("feature/login"))
}

func TestRemoveAutoDetectsRepoFromManagedWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/auto-remove"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.mergeWorktreeBranch(t, branchName)

	result := testRepository.runTimberFrom(t, testRepository.worktreePath(branchName), "remove", branchName)
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}
