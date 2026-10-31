package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHooksCommitMsgAddsChangeID(t *testing.T) {
	t.Parallel()
	repository := newCommitMessageRepository(t)
	messagePath := filepath.Join(repository, "message")
	require.NoError(t, os.WriteFile(messagePath, []byte("Add feature\n"), 0o600))

	result := runTimberFromWithRuntime(t, testRuntime(t), repository, "hooks", "commit-msg", "message")
	require.NoError(t, result.err, result.stderr)
	assert.Empty(t, result.stdout)
	assertChangeID(t, messagePath)
}

func TestReviewCommitMsgRemainsHiddenAlias(t *testing.T) {
	t.Parallel()
	repository := newCommitMessageRepository(t)
	messagePath := filepath.Join(repository, "message")
	require.NoError(t, os.WriteFile(messagePath, []byte("Add feature\n"), 0o600))

	runtime := testRuntime(t)
	aliased := runTimberFromWithRuntime(t, runtime, repository, "review", "commit-msg", "message")
	require.NoError(t, aliased.err, aliased.stderr)
	assertChangeID(t, messagePath)

	hooks := runTimberFromWithRuntime(t, runtime, repository, "hooks", "--help")
	require.NoError(t, hooks.err, hooks.stderr)
	assert.Contains(t, hooks.stdout, "commit-msg")

	review := runTimberFromWithRuntime(t, runtime, repository, "review", "--help")
	require.NoError(t, review.err, review.stderr)
	assert.NotContains(t, review.stdout, "commit-msg")
}

func newCommitMessageRepository(t *testing.T) string {
	t.Helper()
	repository := resolvedTempDir(t)
	runGitCommand(t, repository, "init", "-b", "main")
	runGitCommand(t, repository, "config", "user.name", "Test User")
	runGitCommand(t, repository, "config", "user.email", "test@example.com")
	runGitCommand(t, repository, "commit", "--allow-empty", "-m", "Base")
	return repository
}

func assertChangeID(t *testing.T, messagePath string) {
	t.Helper()
	message, err := os.ReadFile(messagePath)
	require.NoError(t, err)
	assert.Regexp(t, `(?m)^Change-Id: I[0-9a-f]{40}$`, string(message))
}
