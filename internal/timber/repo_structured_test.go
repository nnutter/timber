package timber

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStructuredRepoRenameMovesWorktreeHierarchy(t *testing.T) {
	t.Parallel()
	const branchName = "feature/structured"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	result := testRepository.runTimber(t, "repo", "rename", testRepoName, "nnutter/timber")
	require.NoError(t, result.err, result.stderr)

	runtime := testRepository.runtime
	assert.DirExists(t, runtime.bareRepoPath("nnutter/timber"))
	assert.DirExists(t, runtime.managedWorktreePath("nnutter/timber", branchName))
	assert.NoDirExists(t, testRepository.barePath)
	assert.NoDirExists(t, testRepository.worktreePath(branchName))

	listResult := runTimberCommandWithRuntime(t, runtime, "list", at("nnutter/timber", ""))
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, branchName)

	back := runTimberCommandWithRuntime(t, runtime, "repo", "rename", "nnutter/timber", "timber")
	require.NoError(t, back.err, back.stderr)
	assert.DirExists(t, runtime.bareRepoPath("timber"))
	assert.DirExists(t, runtime.managedWorktreePath("timber", branchName))
	// Empty grouping directories are pruned on the way back to a simple name.
	assert.NoDirExists(t, filepath.Join(runtime.reposDirectory(), "nnutter"))
	assert.NoDirExists(t, filepath.Join(runtime.WorktreeRoot, "nnutter"))
}

func TestStructuredRepoRemovePrunesEmptyParents(t *testing.T) {
	t.Parallel()
	fixture := newTestRepository(t)
	registerAdditionalRepo(t, fixture, "acme/timber")

	result := fixture.runTimber(t, "repo", "remove", "acme/timber")
	require.NoError(t, result.err, result.stderr)
	assert.NoDirExists(t, fixture.runtime.bareRepoPath("acme/timber"))
	assert.NoDirExists(t, filepath.Join(fixture.runtime.reposDirectory(), "acme"))
}

func TestStructuredRepoCreateUsesNestedLayout(t *testing.T) {
	t.Parallel()
	fixture := newTestRepository(t)
	registerAdditionalRepo(t, fixture, "acme/timber")

	result := fixture.runTimber(t, "create", at("acme/timber", "feature/nested"))
	require.NoError(t, result.err, result.stderr)
	assert.DirExists(t, fixture.runtime.managedWorktreePath("acme/timber", "feature/nested"))

	quiet := runTimberCommandWithRuntime(t, fixture.runtime, "repo", "list", "-q")
	require.NoError(t, quiet.err, quiet.stderr)
	assert.Contains(t, quiet.stdout, "acme/timber\n")
}
