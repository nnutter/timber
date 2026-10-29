package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoAddListRemove(t *testing.T) {
	t.Parallel()
	runtime := testRuntime(t)
	home := runtime.HomeDirectory

	remotePath := filepath.Join(resolvedTempDir(t), "remote.git")
	runGitCommand(t, resolvedTempDir(t), "init", "--bare", remotePath)
	seedImportRemote(t, remotePath)

	addResult := runTimberCommandWithRuntime(t, runtime, "repo", "add", "--name", "demo", remotePath)
	require.NoError(t, addResult.err, addResult.stderr)
	assert.Contains(t, addResult.stderr, "added repository demo")

	barePath := filepath.Join(runtime.WorktreeRoot, "demo", ".git")
	fetch := strings.TrimSpace(runGitCommand(t, barePath, "config", "--get", "remote.origin.fetch"))
	assert.Equal(t, "+refs/heads/*:refs/remotes/origin/*", fetch)
	originHead := strings.TrimSpace(runGitCommand(t, barePath, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"))
	assert.Equal(t, "origin/main", originHead)

	listResult := runTimberCommandWithRuntime(t, runtime, "repo", "list")
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, "Name")
	assert.NotContains(t, listResult.stdout, "Path")
	assert.Contains(t, listResult.stdout, "Origin")
	assert.Contains(t, listResult.stdout, "demo")
	assert.NotContains(t, listResult.stdout, filepath.Join("~", "worktrees", "demo", ".git"))
	assert.Contains(t, listResult.stdout, remotePath)
	assert.NotContains(t, listResult.stdout, home)

	removeResult := runTimberCommandWithRuntime(t, runtime, "repo", "remove", "demo")
	require.NoError(t, removeResult.err, removeResult.stderr)

	listAfter := runTimberCommandWithRuntime(t, runtime, "repo", "list")
	require.NoError(t, listAfter.err)
	assert.Contains(t, listAfter.stdout, "Name")
	assert.NotContains(t, listAfter.stdout, "Path")
	assert.Contains(t, listAfter.stdout, "Origin")
	assert.NotContains(t, listAfter.stdout, "demo")
}
