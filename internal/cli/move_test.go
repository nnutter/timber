package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoveRelocatesWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/move"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	destination := filepath.Join(resolvedTempDir(t), "relocated", "checkout")

	result := testRepository.runTimber(t, "move", at(testRepoName, branchName), destination)
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "moved "+branchName+" to "+destination)

	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
	testRepository.assertPathPresent(t, destination)

	switched := testRepository.runTimber(t, "switch", branchName)
	require.NoError(t, switched.err, switched.stderr)
	assert.Equal(t, destination, strings.TrimSpace(switched.stdout))
}

func TestMovePreservesWorktreeNameForListAndPrune(t *testing.T) {
	t.Parallel()
	const branchName = "feature/move-name"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.mergeWorktreeBranch(t, branchName)
	destination := filepath.Join(resolvedTempDir(t), "checkout")

	result := testRepository.runTimber(t, "move", branchName, destination)
	require.NoError(t, result.err, result.stderr)

	listed := testRepository.runTimber(t, "list", "--json")
	require.NoError(t, listed.err, listed.stderr)
	assert.Contains(t, listed.stdout, `"name":"`+branchName+`"`)
	assert.Contains(t, listed.stdout, destination)

	pruned := testRepository.runTimber(t, "prune", "--dry-run")
	require.NoError(t, pruned.err, pruned.stderr)
	assert.Contains(t, pruned.stderr, "would prune "+branchName)
}

func TestMoveReportsMovedCurrentDirectory(t *testing.T) {
	t.Parallel()
	const branchName = "feature/move-current"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	subdirectory := filepath.Join(testRepository.worktreePath(branchName), "nested")
	require.NoError(t, os.MkdirAll(subdirectory, 0o755))
	destination := filepath.Join(resolvedTempDir(t), "moved-current")
	pathFile := filepath.Join(resolvedTempDir(t), "moved-path")
	runtime := testRepository.runtime
	runtime.CurrentDirectory = subdirectory
	runtime.RenamePathFile = pathFile
	runtime.TemporaryDirectory = filepath.Dir(pathFile)

	result := runTimberCommandWithRuntime(t, runtime, "move", at(testRepoName, branchName), destination)
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "current directory moved")

	contents, err := os.ReadFile(pathFile)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(destination, "nested")+"\n", string(contents))
}

func TestMoveRejectsExistingDestination(t *testing.T) {
	t.Parallel()
	const branchName = "feature/move-conflict"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	destination := filepath.Join(resolvedTempDir(t), "occupied")
	require.NoError(t, os.MkdirAll(destination, 0o755))

	result := testRepository.runTimber(t, "move", branchName, destination)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "already exists")
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
}

func TestMoveRejectsDestinationInsideWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/move-nested"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	destination := filepath.Join(testRepository.worktreePath(branchName), "inner")

	result := testRepository.runTimber(t, "move", branchName, destination)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "into itself")
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
}

func TestMoveCompletionOffersWorktreeNames(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/a")).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/b")).err)

	stdout := runCompleteWithRuntime(t, testRepository.runtime, "move", "")
	assert.Contains(t, stdout, "feature/a")
	assert.Contains(t, stdout, "feature/b")

	stdout = runCompleteWithRuntime(t, testRepository.runtime, "move", "feature/a", "")
	assert.NotContains(t, stdout, "feature/a")
}
