package timber

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitPassesThroughToWorktreeGitDir(t *testing.T) {
	t.Parallel()

	const branchName = "feature/git-passthrough"
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	worktreePath := testRepository.worktreePath(branchName)
	expectedGitDir := strings.TrimSpace(runGitCommand(t, worktreePath, "rev-parse", "--absolute-git-dir"))

	result := runTimberFromWithRuntime(t, testRepository.runtime, worktreePath, "git", "rev-parse", "--absolute-git-dir")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, expectedGitDir, strings.TrimSpace(result.stdout))
}

func TestGitPassesThroughFlagsWithoutParsing(t *testing.T) {
	t.Parallel()

	const branchName = "feature/git-flags"
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	worktreePath := testRepository.worktreePath(branchName)

	result := runTimberFromWithRuntime(t, testRepository.runtime, worktreePath, "git", "--version")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, "git version")
}

func TestGitStripsLeadingDashDash(t *testing.T) {
	t.Parallel()

	const branchName = "feature/git-dashdash"
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	worktreePath := testRepository.worktreePath(branchName)
	expectedGitDir := strings.TrimSpace(runGitCommand(t, worktreePath, "rev-parse", "--absolute-git-dir"))

	result := runTimberFromWithRuntime(t, testRepository.runtime, worktreePath, "git", "--", "rev-parse", "--absolute-git-dir")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, expectedGitDir, strings.TrimSpace(result.stdout))
}

func TestGitReportsGitFailure(t *testing.T) {
	t.Parallel()

	const branchName = "feature/git-failure"
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	worktreePath := testRepository.worktreePath(branchName)

	result := runTimberFromWithRuntime(t, testRepository.runtime, worktreePath, "git", "rev-parse", "--verify", "refs/heads/missing")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "run git")
}

func TestGitFailsOutsideWorktree(t *testing.T) {
	t.Parallel()

	testRepository := newTestRepository(t)

	result := runTimberFromWithRuntime(t, testRepository.runtime, testRepository.home, "git", "status")
	require.ErrorContains(t, result.err, "not inside a worktree")
	assert.NotContains(t, result.err.Error(), "rev-parse")
	assert.NotContains(t, result.err.Error(), "exit status")
}

func TestGitFailsInBareRepository(t *testing.T) {
	t.Parallel()

	testRepository := newTestRepository(t)

	result := runTimberFromWithRuntime(t, testRepository.runtime, testRepository.barePath, "git", "status")
	require.ErrorContains(t, result.err, "not inside a worktree")
}
