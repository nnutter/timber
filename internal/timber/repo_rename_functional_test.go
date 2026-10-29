package timber

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoRenameRollsBackCompletedWorktreeMoves(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	for _, branchName := range []string{"feature/first", "feature/second"} {
		_, _, err := testRepository.runtime.CreateManagedWorktree(t.Context(), &bytes.Buffer{}, io.Discard, "", at(testRepoName, branchName), "", false, "", false, false)
		require.NoError(t, err)
	}

	renameCalls := 0
	renamePath := func(source string, destination string) error {
		renameCalls++
		if renameCalls == 2 {
			return fmt.Errorf("injected rename failure")
		}
		return os.Rename(source, destination)
	}
	repair := func(barePath string, worktreePaths []string) error {
		return repairWorktrees(testRepository.runtime, barePath, worktreePaths)
	}

	plan, err := testRepository.runtime.buildRepositoryRenamePlan(testRepoName, "renamed")
	require.NoError(t, err)
	err = plan.apply(renamePath, repair)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected rename failure")
	assert.DirExists(t, testRepository.barePath)
	assert.NoDirExists(t, testRepository.runtime.bareRepoPath("renamed"))
	for _, branchName := range []string{"feature/first", "feature/second"} {
		assert.DirExists(t, testRepository.worktreePath(branchName))
		_, openErr := openRepository(testRepository.runtime, testRepository.worktreePath(branchName))
		require.NoError(t, openErr)
	}
}

func TestRepoRenameRollsBackAfterRepairFailure(t *testing.T) {
	t.Parallel()
	const branchName = "feature/repair-failure"

	testRepository := newTestRepository(t)
	_, _, err := testRepository.runtime.CreateManagedWorktree(t.Context(), &bytes.Buffer{}, io.Discard, "", at(testRepoName, branchName), "", false, "", false, false)
	require.NoError(t, err)

	repairCalls := 0
	repair := func(barePath string, worktreePaths []string) error {
		repairCalls++
		if repairCalls == 1 {
			return fmt.Errorf("injected repair failure")
		}
		return repairWorktrees(testRepository.runtime, barePath, worktreePaths)
	}

	plan, err := testRepository.runtime.buildRepositoryRenamePlan(testRepoName, "renamed")
	require.NoError(t, err)
	err = plan.apply(os.Rename, repair)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected repair failure")
	assert.DirExists(t, testRepository.barePath)
	assert.NoDirExists(t, testRepository.runtime.bareRepoPath("renamed"))
	assert.DirExists(t, testRepository.worktreePath(branchName))
	_, openErr := openRepository(testRepository.runtime, testRepository.worktreePath(branchName))
	require.NoError(t, openErr)
}
