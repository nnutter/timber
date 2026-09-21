package timber

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTUICreateCreatesSelectedWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/ui-create"

	testRepository := newTestRepository(t)
	prompter := &stubCreateWizardPrompter{
		selection: createWizardSelection{repoName: testRepoName, worktreeName: branchName},
	}
	options := &tuiCreateCommandOptions{runtime: testRepository.runtime}
	result := runTUICreate(t, options, prompter)

	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Contains(t, result.stdout, testRepository.worktreePath(branchName))
	require.Len(t, prompter.repos, 1)
	assert.Equal(t, testRepoName, prompter.repos[0].Name)
}

func TestTUICreateCancelDoesNotCreateWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/ui-cancel"

	testRepository := newTestRepository(t)
	options := &tuiCreateCommandOptions{runtime: testRepository.runtime}
	result := runTUICreate(t, options, &stubCreateWizardPrompter{
		selection: createWizardSelection{cancelled: true},
	})

	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
	assert.Empty(t, result.stdout)
}

func TestTUICreateFailsWhenNoRepositoriesAreRegistered(t *testing.T) {
	t.Parallel()
	home := resolvedTempDir(t)
	options := &tuiCreateCommandOptions{runtime: testRuntimeForHome(home, home)}

	result := runTUICreate(t, options, &stubCreateWizardPrompter{})
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "no registered repositories")
}

func TestTUICreateListsEveryRepositoryFromAManagedWorktree(t *testing.T) {
	t.Parallel()
	const currentBranch = "feature/current"
	const createdBranch = "topic/from-other"
	const secondaryName = "other"

	testRepository := newTestRepository(t)
	registerAdditionalRepo(t, testRepository, secondaryName)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, currentBranch)).err)

	prompter := &stubCreateWizardPrompter{
		selection: createWizardSelection{repoName: secondaryName, worktreeName: createdBranch},
	}
	options := new(tuiCreateCommandOptions)
	options.runtime = testRepository.runtime
	options.runtime.CurrentDirectory = testRepository.worktreePath(currentBranch)
	result := runTUICreate(t, options, prompter)

	require.NoError(t, result.err, result.stderr)
	secondaryPath := filepath.Join(testRepository.worktreeRoot, secondaryName, createdBranch, secondaryName)
	_, statErr := os.Stat(secondaryPath)
	require.NoError(t, statErr)
	testRepository.assertPathMissing(t, testRepository.worktreePath(createdBranch))
	repoNames := make([]string, 0, len(prompter.repos))
	for _, repo := range prompter.repos {
		repoNames = append(repoNames, repo.Name)
	}
	assert.Equal(t, []string{secondaryName, testRepoName}, repoNames)
}

func TestTUICreateWithHerdrOpensStandardHerdrSpace(t *testing.T) {
	t.Parallel()
	const branchName = "feature/ui-herdr"

	testRepository := newTestRepository(t)
	logPath := filepath.Join(resolvedTempDir(t), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime.GhExecutable = installFakeGh(t)

	options := new(tuiCreateCommandOptions)
	options.runtime = testRepository.runtime
	options.herdr = true
	result := runTUICreate(t, options, &stubCreateWizardPrompter{
		selection: createWizardSelection{repoName: testRepoName, worktreeName: branchName},
	})

	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Contains(t, result.stderr, "opened herdr space for "+branchName)
	assert.Len(t, readFakeHerdrLog(t, logPath), 11)
}

func TestTUICreateOpensSelectedWorktreeInHerdrSpace(t *testing.T) {
	t.Parallel()
	const branchName = "feature/ui-open"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	logPath := filepath.Join(resolvedTempDir(t), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime.GhExecutable = installFakeGh(t)

	prompter := &stubCreateWizardPrompter{
		selection: createWizardSelection{
			action:       wizardActionOpen,
			repoName:     testRepoName,
			worktreeName: branchName,
		},
	}
	options := &tuiCreateCommandOptions{runtime: testRepository.runtime}
	result := runTUICreate(t, options, prompter)

	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "opened herdr space for "+branchName)
	assert.Len(t, readFakeHerdrLog(t, logPath), 11)
	require.Len(t, prompter.worktrees, 1)
	assert.Equal(t, testRepoName, prompter.worktrees[0].Repo)
	assert.Equal(t, branchName, prompter.worktrees[0].Name)
}

func TestTUICreateWithNoHerdrDoesNotInvokeHerdr(t *testing.T) {
	t.Parallel()
	const branchName = "feature/ui-no-herdr"

	testRepository := newTestRepository(t)
	testRepository.runtime.HerdrEnvironment = true
	logPath := filepath.Join(resolvedTempDir(t), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime.GhExecutable = installFakeGh(t)

	options := new(tuiCreateCommandOptions)
	options.runtime = testRepository.runtime
	options.noHerdr = true
	result := runTUICreate(t, options, &stubCreateWizardPrompter{
		selection: createWizardSelection{repoName: testRepoName, worktreeName: branchName},
	})

	require.NoError(t, result.err, result.stderr)
	_, err := os.Stat(logPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestTUICreateRejectsHerdrAndNoHerdr(t *testing.T) {
	t.Parallel()
	result := runTimberCommand(t, "tui", "--herdr", "--no-herdr")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "if any flags in the group [herdr no-herdr] are set none of the others can be")
}
