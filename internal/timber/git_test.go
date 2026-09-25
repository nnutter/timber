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

func TestGitRunsInNamedWorktreeFromOutside(t *testing.T) {
	t.Parallel()

	const branchA = "feature/git-target"
	const branchB = "feature/git-other"
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchA)).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchB)).err)

	targetPath := testRepository.worktreePath(branchA)
	result := runTimberFromWithRuntime(t, testRepository.runtime, testRepository.home, "git", branchA, "rev-parse", "--show-toplevel")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, targetPath, strings.TrimSpace(result.stdout))
}

func TestGitRunsInNamedWorktreeFromAnotherWorktree(t *testing.T) {
	t.Parallel()

	const branchA = "feature/git-from-a"
	const branchB = "feature/git-from-b"
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchA)).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchB)).err)

	targetPath := testRepository.worktreePath(branchA)
	otherPath := testRepository.worktreePath(branchB)
	result := runTimberFromWithRuntime(t, testRepository.runtime, otherPath, "git", branchA, "rev-parse", "--show-toplevel")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, targetPath, strings.TrimSpace(result.stdout))
}

func TestGitRunsInQualifiedWorktree(t *testing.T) {
	t.Parallel()

	const branchName = "feature/git-shared"
	testRepository := newTestRepository(t)
	otherRepo := "other"
	registerAdditionalRepo(t, testRepository, otherRepo)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(otherRepo, branchName)).err)

	expectedPath := testRepository.worktreePath(branchName)
	result := runTimberFromWithRuntime(t, testRepository.runtime, testRepository.home, "git", at(testRepoName, branchName), "rev-parse", "--show-toplevel")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, expectedPath, strings.TrimSpace(result.stdout))
}

func TestGitAmbiguousBareNameRequiresQualifier(t *testing.T) {
	t.Parallel()

	const branchName = "feature/git-ambiguous"
	testRepository := newTestRepository(t)
	otherRepo := "other"
	registerAdditionalRepo(t, testRepository, otherRepo)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(otherRepo, branchName)).err)

	result := runTimberFromWithRuntime(t, testRepository.runtime, testRepository.home, "git", branchName, "status")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "multiple repositories")
}

func TestGitSelectorStripsFollowingDashDash(t *testing.T) {
	t.Parallel()

	const branchName = "feature/git-separator"
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	targetPath := testRepository.worktreePath(branchName)
	result := runTimberFromWithRuntime(t, testRepository.runtime, testRepository.home, "git", branchName, "--", "rev-parse", "--show-toplevel")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, targetPath, strings.TrimSpace(result.stdout))
}

func TestGitLeadingDashDashForcesCurrentDirectory(t *testing.T) {
	t.Parallel()

	const branchName = "feature/git-forced"
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	result := runTimberFromWithRuntime(t, testRepository.runtime, testRepository.home, "git", "--", branchName, "rev-parse", "--show-toplevel")
	require.ErrorContains(t, result.err, "not inside a worktree")
}

func TestGitQualifiedSelectorReturnsResolutionError(t *testing.T) {
	t.Parallel()

	testRepository := newTestRepository(t)

	result := runTimberFromWithRuntime(t, testRepository.runtime, testRepository.home, "git", at(testRepoName, "feature/git-missing"), "status")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "unknown worktree")
}

func TestGitFallsBackToCurrentForGitSubcommand(t *testing.T) {
	t.Parallel()

	const branchA = "feature/git-current-a"
	const branchB = "feature/git-current-b"
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchA)).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchB)).err)

	currentPath := testRepository.worktreePath(branchB)
	result := runTimberFromWithRuntime(t, testRepository.runtime, currentPath, "git", "rev-parse", "--show-toplevel")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, currentPath, strings.TrimSpace(result.stdout))
}
