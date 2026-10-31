package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateConfiguresPerWorktreeHooks(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	sharedHooks := filepath.Join(repository.barePath, "hooks")
	require.NoError(t, os.MkdirAll(sharedHooks, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sharedHooks, "pre-commit"), []byte("#!/bin/sh\nexit 0\n"), 0o755))

	const branchName = "feature/hooks"
	created := repository.runTimber(t, "create", at(testRepoName, branchName))
	require.NoError(t, created.err, created.stderr)

	worktreePath := repository.worktreePath(branchName)
	gitDir := strings.TrimSpace(runGitCommand(t, worktreePath, "rev-parse", "--absolute-git-dir"))
	hooksPath := filepath.Join(gitDir, "hooks")
	assert.Equal(t, hooksPath, strings.TrimSpace(runGitCommand(t, worktreePath, "config", "--get", "core.hooksPath")))

	info, err := os.Stat(filepath.Join(hooksPath, "pre-commit"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&0o100, "copied hook must stay executable")

	// Git stops special-casing core.bare once per-worktree configuration is on.
	assert.Equal(t, "true", strings.TrimSpace(runGitCommand(t, repository.barePath, "rev-parse", "--is-bare-repository")))
	assert.Contains(t, runGitCommand(t, worktreePath, "status", "--porcelain=v2", "--branch"), "# branch.head "+branchName)
}

func TestCreateKeepsExistingHooksPath(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	customHooks := filepath.Join(repository.home, "hooks")
	runGitCommand(t, repository.barePath, "config", "core.hooksPath", customHooks)

	const branchName = "feature/custom-hooks"
	created := repository.runTimber(t, "create", at(testRepoName, branchName))
	require.NoError(t, created.err, created.stderr)

	worktreePath := repository.worktreePath(branchName)
	assert.Equal(t, customHooks, strings.TrimSpace(runGitCommand(t, worktreePath, "config", "--get", "core.hooksPath")))

	_, err := gitCommand(t, repository.barePath, "config", "--get", "extensions.worktreeConfig").Output()
	assert.Error(t, err, "an existing hooks path must not change repository configuration")
}
