package timber

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPruneDryRunWithPromptListsSelectedWorktrees(t *testing.T) {
	t.Parallel()
	const branchName = "feature/prompt-dry-run"

	testRepository := newTestRepository(t)
	createPruneTestWorktree(t, testRepository.runtime, testRepoName, branchName)
	testRepository.commitFileInWorktree(t, branchName, "extra.txt", "extra\n")

	var stderr bytes.Buffer
	err := testRepository.runtime.PruneWorktrees(
		nil,
		&stderr,
		at(testRepoName, ""),
		true,
		true,
		stubPrompter{selected: []WorktreeRef{{Name: branchName, Repo: testRepoName}}},
	)
	require.NoError(t, err, stderr.String())
	assert.Contains(t, stderr.String(), "would prune "+branchName+" ("+testRepoName+")")
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
}

func TestPrunePromptDistinguishesSameNameInTwoRepos(t *testing.T) {
	t.Parallel()
	const branchName = "feature/same"

	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)

	createPruneTestWorktree(t, primary.runtime, testRepoName, branchName)
	createPruneTestWorktree(t, primary.runtime, secondaryName, branchName)

	var stderr bytes.Buffer
	require.NoError(t, primary.runtime.PruneWorktrees(
		nil,
		&stderr,
		"",
		true,
		false,
		stubPrompter{selected: []WorktreeRef{{Repo: secondaryName, Name: branchName}}},
	), stderr.String())

	primary.assertPathPresent(t, primary.worktreePath(branchName))
	_, err := os.Stat(filepath.Join(primary.worktreeRoot, secondaryName, branchName, secondaryName))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestPrunePromptCanForceRemoveSelectedWorktrees(t *testing.T) {
	t.Parallel()
	const branchName = "feature/prompt"

	testRepository := newTestRepository(t)
	createPruneTestWorktree(t, testRepository.runtime, testRepoName, branchName)
	testRepository.commitFileInWorktree(t, branchName, "extra.txt", "extra\n")

	var stderr bytes.Buffer
	err := testRepository.runtime.PruneWorktrees(
		nil,
		&stderr,
		at(testRepoName, ""),
		true,
		false,
		stubPrompter{selected: []WorktreeRef{{Name: branchName}}},
	)
	require.NoError(t, err, stderr.String())
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}

func createPruneTestWorktree(t *testing.T, runtime Runtime, repo, branch string) {
	t.Helper()
	_, _, err := runtime.CreateManagedWorktree(t.Context(), &bytes.Buffer{}, io.Discard, "", at(repo, branch), "", false, "", false, false)
	require.NoError(t, err)
}
