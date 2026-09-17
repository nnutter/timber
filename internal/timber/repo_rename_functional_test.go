package timber

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoRenameMovesManagedWorktreesAndPreservesUnmanagedWorktrees(t *testing.T) {
	t.Parallel()
	const (
		branchName  = "feature/rename/nested"
		newRepoName = "renamed"
	)

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.writeFileInWorktree(t, branchName, "dirty.txt", "dirty\n")

	unmanagedPath := filepath.Join(resolvedTempDir(t), "unmanaged")
	runGitCommand(t, testRepository.barePath, "branch", "unmanaged", "main")
	runGitCommand(t, testRepository.barePath, "worktree", "add", unmanagedPath, "unmanaged")
	detachedPath := filepath.Join(resolvedTempDir(t), "detached")
	runGitCommand(t, testRepository.barePath, "worktree", "add", "--detach", detachedPath, "main")

	result := testRepository.runTimber(t, "repo", "rename", testRepoName, newRepoName)
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "renamed repository repo to renamed")

	newBarePath := testRepository.runtime.bareRepoPath(newRepoName)
	newWorktreePath := testRepository.runtime.managedWorktreePath(newRepoName, branchName)
	assert.NoDirExists(t, testRepository.barePath)
	assert.DirExists(t, newBarePath)
	assert.NoDirExists(t, testRepository.worktreePath(branchName))
	assert.DirExists(t, newWorktreePath)
	assert.FileExists(t, filepath.Join(newWorktreePath, "dirty.txt"))
	assert.DirExists(t, unmanagedPath)
	assert.DirExists(t, detachedPath)

	managedRepository, err := openRepository(testRepository.runtime, newWorktreePath)
	require.NoError(t, err)
	managedCommonDir, err := managedRepository.commonGitDir()
	require.NoError(t, err)
	managedCommonDirMatches, err := samePath(managedCommonDir, newBarePath)
	require.NoError(t, err)
	assert.True(t, managedCommonDirMatches)

	unmanagedRepository, err := openRepository(testRepository.runtime, unmanagedPath)
	require.NoError(t, err)
	unmanagedCommonDir, err := unmanagedRepository.commonGitDir()
	require.NoError(t, err)
	unmanagedCommonDirMatches, err := samePath(unmanagedCommonDir, newBarePath)
	require.NoError(t, err)
	assert.True(t, unmanagedCommonDirMatches)

	detachedRepository, err := openRepository(testRepository.runtime, detachedPath)
	require.NoError(t, err)
	detachedCommonDir, err := detachedRepository.commonGitDir()
	require.NoError(t, err)
	detachedCommonDirMatches, err := samePath(detachedCommonDir, newBarePath)
	require.NoError(t, err)
	assert.True(t, detachedCommonDirMatches)

	listResult := runTimberCommandWithRuntime(t, testRepository.runtime, "list", at(newRepoName, ""))
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, branchName)
}

func TestRepoRenameWithoutWorktrees(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	result := testRepository.runTimber(t, "repo", "rename", testRepoName, "renamed")
	require.NoError(t, result.err, result.stderr)
	assert.NoDirExists(t, testRepository.barePath)
	assert.DirExists(t, testRepository.runtime.bareRepoPath("renamed"))

	oldResult := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.Error(t, oldResult.err)
	assert.Contains(t, oldResult.err.Error(), `unknown repository "repo"`)
}

func TestRepoRenameReportsMovedCurrentDirectory(t *testing.T) {
	t.Parallel()
	const branchName = "feature/current-rename"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	subdirectory := filepath.Join(testRepository.worktreePath(branchName), "nested")
	require.NoError(t, os.MkdirAll(subdirectory, 0o755))
	pathFile := filepath.Join(resolvedTempDir(t), "renamed-path")
	runtime := testRepository.runtime
	runtime.CurrentDirectory = subdirectory
	runtime.RenamePathFile = pathFile

	result := runTimberCommandWithRuntime(t, runtime, "repo", "rename", testRepoName, "renamed")
	require.NoError(t, result.err, result.stderr)

	contents, err := os.ReadFile(pathFile)
	require.NoError(t, err)
	assert.Equal(t, testRepository.runtime.managedWorktreePath("renamed", branchName)+string(filepath.Separator)+"nested\n", string(contents))
}

func TestRepoRenameRejectsInvalidAndConflictingNames(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		newName   string
		prepare   func(*testing.T, testRepository)
		wantError string
	}{
		{
			name:      "same name",
			newName:   testRepoName,
			prepare:   func(*testing.T, testRepository) {},
			wantError: "already named",
		},
		{
			name:      "invalid name",
			newName:   "invalid/name",
			prepare:   func(*testing.T, testRepository) {},
			wantError: "must not contain path separators",
		},
		{
			name:    "repository collision",
			newName: "existing",
			prepare: func(t *testing.T, testRepository testRepository) {
				require.NoError(t, os.MkdirAll(testRepository.runtime.bareRepoPath("existing"), 0o755))
			},
			wantError: "already exists",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testRepository := newTestRepository(t)
			testCase.prepare(t, testRepository)

			result := testRepository.runTimber(t, "repo", "rename", testRepoName, testCase.newName)
			require.Error(t, result.err)
			assert.Contains(t, result.err.Error(), testCase.wantError)
			assert.DirExists(t, testRepository.barePath)
		})
	}
}

func TestRepoRenameRejectsUnknownRepository(t *testing.T) {
	t.Parallel()
	newTestRepository(t)

	result := runTimberCommand(t, "repo", "rename", "missing", "renamed")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), `unknown repository "missing"`)
}

func TestRepoRenameRejectsPrunableWorktree(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	prunablePath := filepath.Join(resolvedTempDir(t), "prunable")
	runGitCommand(t, testRepository.barePath, "branch", "prunable", "main")
	runGitCommand(t, testRepository.barePath, "worktree", "add", prunablePath, "prunable")
	require.NoError(t, os.RemoveAll(prunablePath))

	result := testRepository.runTimber(t, "repo", "rename", testRepoName, "renamed")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "prunable")
	assert.DirExists(t, testRepository.barePath)
	assert.NoDirExists(t, testRepository.runtime.bareRepoPath("renamed"))
}

func TestRepoRenameRejectsWorktreeDestinationCollision(t *testing.T) {
	t.Parallel()
	const branchName = "feature/collision"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	require.NoError(t, os.MkdirAll(testRepository.runtime.managedWorktreePath("renamed", branchName), 0o755))

	result := testRepository.runTimber(t, "repo", "rename", testRepoName, "renamed")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "worktree directory")
	assert.DirExists(t, testRepository.barePath)
	assert.DirExists(t, testRepository.worktreePath(branchName))
}

func TestRepoRenameRollsBackCompletedWorktreeMoves(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	for _, branchName := range []string{"feature/first", "feature/second"} {
		require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	}

	renameCalls := 0
	options := &repoRenameCommandOptions{
		runtime: testRepository.runtime,
		renamePath: func(source string, destination string) error {
			renameCalls++
			if renameCalls == 2 {
				return fmt.Errorf("injected rename failure")
			}
			return os.Rename(source, destination)
		},
		repairWorktrees: func(barePath string, worktreePaths []string) error {
			return repairWorktrees(testRepository.runtime, barePath, worktreePaths)
		},
	}
	command := newTestRootCommandWithRuntime(t, testRepository.runtime)
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)

	err := options.Execute(command, []string{testRepoName, "renamed"})
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
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	repairCalls := 0
	options := &repoRenameCommandOptions{
		runtime:    testRepository.runtime,
		renamePath: os.Rename,
		repairWorktrees: func(barePath string, worktreePaths []string) error {
			repairCalls++
			if repairCalls == 1 {
				return fmt.Errorf("injected repair failure")
			}
			return repairWorktrees(testRepository.runtime, barePath, worktreePaths)
		},
	}
	command := newTestRootCommandWithRuntime(t, testRepository.runtime)
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)

	err := options.Execute(command, []string{testRepoName, "renamed"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected repair failure")
	assert.DirExists(t, testRepository.barePath)
	assert.NoDirExists(t, testRepository.runtime.bareRepoPath("renamed"))
	assert.DirExists(t, testRepository.worktreePath(branchName))
	_, openErr := openRepository(testRepository.runtime, testRepository.worktreePath(branchName))
	require.NoError(t, openErr)
}

func TestRepoRenameCompletionOffersRegisteredReposOnlyForOldName(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	oldNameCompletion := runCompleteWithRuntime(t, testRepository.runtime, "repo", "rename", "")
	assert.Contains(t, oldNameCompletion, testRepoName)

	newNameCompletion := runCompleteWithRuntime(t, testRepository.runtime, "repo", "rename", testRepoName, "")
	assert.NotContains(t, newNameCompletion, testRepoName)
	assert.Contains(t, newNameCompletion, ":4")
}
