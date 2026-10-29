package timber

import (
	"bytes"
	"errors"
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
	var candidate WorktreeRef
	err := testRepository.runtime.PruneWorktrees(
		nil,
		&stderr,
		at(testRepoName, ""),
		true,
		true,
		stubPrompter{selected: []WorktreeRef{{Name: branchName, Repo: testRepoName}}},
		PruneReporters{DryRun: func(_ io.Writer, name, repo, path string) error {
			candidate = WorktreeRef{Name: name, Repo: repo, DisplayPath: path}
			return nil
		}},
	)
	require.NoError(t, err, stderr.String())
	assert.Equal(t, WorktreeRef{Name: branchName, Repo: testRepoName, DisplayPath: testRepository.runtime.displayHomePath(testRepository.worktreePath(branchName))}, candidate)
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
		PruneReporters{Removal: func(io.Writer, string, string, string, bool) error { return nil }},
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
		PruneReporters{Removal: func(io.Writer, string, string, string, bool) error { return nil }},
	)
	require.NoError(t, err, stderr.String())
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}

func TestPruneStopsAfterReportingError(t *testing.T) {
	t.Parallel()
	for _, dryRun := range []bool{true, false} {
		name := "removal"
		if dryRun {
			name = "dry run"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repo := newTestRepository(t)
			first, second := "feature/first", "feature/second"
			createPruneTestWorktree(t, repo.runtime, testRepoName, first)
			createPruneTestWorktree(t, repo.runtime, testRepoName, second)
			writeErr := errors.New("write failed")
			var reported []string
			reporters := PruneReporters{
				DryRun: func(_ io.Writer, name, _, _ string) error {
					reported = append(reported, name)
					return writeErr
				},
				Removal: func(_ io.Writer, name, _, _ string, _ bool) error {
					reported = append(reported, name)
					return writeErr
				},
			}
			prompter := stubPrompter{selected: []WorktreeRef{{Name: first}, {Name: second}}}
			err := repo.runtime.PruneWorktrees(nil, io.Discard, at(testRepoName, ""), true, dryRun, prompter, reporters)
			require.ErrorIs(t, err, writeErr)
			assert.Equal(t, []string{first}, reported)
			if dryRun {
				repo.assertPathPresent(t, repo.worktreePath(first))
			} else {
				repo.assertPathMissing(t, repo.worktreePath(first))
			}
			repo.assertPathPresent(t, repo.worktreePath(second))
		})
	}
}

func TestPruneRequiresReporterBeforeRemoval(t *testing.T) {
	t.Parallel()
	repo := newTestRepository(t)
	const branch = "feature/reporter"
	createPruneTestWorktree(t, repo.runtime, testRepoName, branch)
	err := repo.runtime.PruneWorktrees(nil, io.Discard, at(testRepoName, ""), false, false, nil, PruneReporters{})
	require.ErrorContains(t, err, "prune removal requires a reporter")
	repo.assertPathPresent(t, repo.worktreePath(branch))
}

func createPruneTestWorktree(t *testing.T, runtime Runtime, repo, branch string) {
	t.Helper()
	_, _, err := runtime.CreateManagedWorktree(t.Context(), &bytes.Buffer{}, io.Discard, "", at(repo, branch), "", false, "", false, false, testCreateReporters())
	require.NoError(t, err)
}
