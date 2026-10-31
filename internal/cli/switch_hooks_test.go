package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSwitchConfiguresPerWorktreeHooks(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	sharedHooks := filepath.Join(repository.barePath, "hooks")
	require.NoError(t, os.MkdirAll(sharedHooks, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sharedHooks, "pre-push"), []byte("#!/bin/sh\nexit 0\n"), 0o755))

	const branchName = "feature/switch-hooks"
	require.NoError(t, repository.runTimber(t, "create", at(testRepoName, branchName)).err)
	worktreePath := repository.worktreePath(branchName)
	gitDir := strings.TrimSpace(runGitCommand(t, worktreePath, "rev-parse", "--absolute-git-dir"))
	hooksPath := filepath.Join(gitDir, "hooks")

	// A worktree from an earlier Timber version has no per-worktree hooks.
	runGitCommand(t, worktreePath, "config", "--worktree", "--unset", "core.hooksPath")
	require.NoError(t, os.RemoveAll(hooksPath))

	switched := repository.runTimber(t, "switch", at(testRepoName, branchName))
	require.NoError(t, switched.err, switched.stderr)
	assert.Equal(t, worktreePath+"\n", switched.stdout)
	assert.Equal(t, hooksPath, strings.TrimSpace(runGitCommand(t, worktreePath, "config", "--get", "core.hooksPath")))
	assert.FileExists(t, filepath.Join(hooksPath, "pre-push"))
}

func TestSwitchFailsWhenHookConfigurationFails(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	const branchName = "feature/broken-hooks"
	require.NoError(t, repository.runTimber(t, "create", at(testRepoName, branchName)).err)
	worktreePath := repository.worktreePath(branchName)
	gitDir := strings.TrimSpace(runGitCommand(t, worktreePath, "rev-parse", "--absolute-git-dir"))

	runGitCommand(t, worktreePath, "config", "--worktree", "--unset", "core.hooksPath")
	require.NoError(t, os.RemoveAll(filepath.Join(gitDir, "hooks")))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "hooks"), []byte("not a directory"), 0o600))

	switched := repository.runTimber(t, "switch", at(testRepoName, branchName))
	require.Error(t, switched.err)
	assert.Empty(t, switched.stdout, "a failed switch must not report a path to change to")
}
