package timber

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandAliases(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"ls"},
		{"clean"},
		{"rm"},
		{"sw"},
		{"repo", "ls"},
		{"repo", "rm"},
		{"repo", "mv"},
	} {
		args = append(args, "--help")
		result := runTimberCommand(t, args...)
		require.NoError(t, result.err, strings.Join(args, " ")+": "+result.stderr)
	}
}

func TestCreateListAndRemoveLifecycle(t *testing.T) {
	t.Parallel()
	const branchName = "feature/one"

	testRepository := newTestRepository(t)

	createResult := testRepository.runTimber(t, "create", at(testRepoName, branchName))
	require.NoError(t, createResult.err, createResult.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Contains(t, createResult.stdout, testRepository.worktreePath(branchName))

	branchCommitHash := strings.TrimSpace(runGitCommand(t, testRepository.barePath, "rev-parse", "--short=7", branchName))

	listResult := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, "Name")
	assert.Contains(t, listResult.stdout, "Repo")
	assert.Less(t, strings.Index(listResult.stdout, "Name"), strings.Index(listResult.stdout, "Repo"))
	assert.Contains(t, listResult.stdout, testRepoName)
	assert.Contains(t, listResult.stdout, branchName)
	// The default upstream stays hidden in Status; only worktrees
	// tracking something else call out their upstream.
	assert.NotContains(t, listResult.stdout, "[origin/main]")
	assert.Contains(t, listResult.stdout, branchCommitHash)

	testRepository.mergeWorktreeBranch(t, branchName)
	mergedCommitHash := strings.TrimSpace(runGitCommand(t, testRepository.barePath, "rev-parse", "--short=7", branchName))

	removeResult := testRepository.runTimber(t, "remove", at(testRepoName, branchName))
	require.NoError(t, removeResult.err, removeResult.stderr)
	assert.Contains(t, removeResult.stderr, mergedCommitHash)

	testRepository.assertBranchMissing(t, branchName)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}
