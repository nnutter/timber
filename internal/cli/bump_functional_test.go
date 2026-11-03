package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bumpTestRepository registers a repository whose runtime carries the
// committer identity that an annotated tag needs.
func bumpTestRepository(t *testing.T) testRepository {
	t.Helper()
	repository := newTestRepository(t)
	repository.runtime = withTestEnvironment(repository.runtime,
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	return repository
}

func TestBumpCreatesAndPushesTheNewestTag(t *testing.T) {
	t.Parallel()
	const branchName = "release"

	repository := bumpTestRepository(t)
	runGitCommand(t, repository.barePath, "tag", "--annotate", "v1.2.3", "--message", "v1.2.3")
	require.NoError(t, repository.runTimber(t, "create", at(testRepoName, branchName)).err)

	result := repository.runTimber(t, "bump", "--patch", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, "v1.2.4\n", result.stdout)
	assert.Equal(t, "v1.2.4", strings.TrimSpace(runGitCommand(t, repository.remotePath, "tag", "--list", "v1.2.4")))
}

func TestBumpFromTheCurrentWorktreeWithoutPush(t *testing.T) {
	t.Parallel()
	const branchName = "release"

	repository := bumpTestRepository(t)
	worktreePath := repository.worktreePath(branchName)
	require.NoError(t, repository.runTimber(t, "create", at(testRepoName, branchName)).err)

	result := repository.runTimberFrom(t, worktreePath, "bump", "--minor", "--no-push")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, "v0.1.0\n", result.stdout)
	assert.Empty(t, strings.TrimSpace(runGitCommand(t, repository.remotePath, "tag", "--list")))
}

func TestBumpRequiresExactlyOneKind(t *testing.T) {
	t.Parallel()
	result := runTimberCommand(t, "bump", "--major", "--minor")
	require.ErrorContains(t, result.err, "exactly one of --major, --minor, or --patch is required")
}
