package cli

import (
	"fmt"
	"os"
	"path/filepath"
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

// installFakeReleaseGh installs a gh stub that prints url for a release.
func installFakeReleaseGh(t *testing.T, url string) string {
	t.Helper()
	scriptPath := filepath.Join(resolvedTempDir(t), "gh")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1 $2" = "release create" ]; then
  printf '%%s\n' %q
  exit 0
fi
echo "unexpected gh call: $*" >&2
exit 1
`, url)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
	return scriptPath
}

func TestBumpReleasePrintsDraftURL(t *testing.T) {
	t.Parallel()
	const branchName = "release"
	const releaseURL = "https://github.com/owner/timber/releases/tag/v0.0.1"

	repository := bumpTestRepository(t)
	repository.runtime.GhExecutable = installFakeReleaseGh(t, releaseURL)
	require.NoError(t, repository.runTimber(t, "create", at(testRepoName, branchName)).err)

	result := repository.runTimber(t, "bump", "--patch", "--release", "--no-open", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, "v0.0.1\n"+releaseURL+"\n", result.stdout)
}

// pushCommitToOriginMain adds a commit to the remote's main branch through a
// separate clone, as another machine would.
func pushCommitToOriginMain(t *testing.T, remotePath string) {
	t.Helper()
	cloneParent := resolvedTempDir(t)
	clonePath := filepath.Join(cloneParent, "clone")
	runGitCommand(t, cloneParent, "clone", remotePath, clonePath)
	require.NoError(t, os.WriteFile(filepath.Join(clonePath, "next.txt"), []byte("next\n"), 0o644))
	runGitCommand(t, clonePath, "add", "next.txt")
	runGitCommand(t, clonePath, "commit", "-m", "next")
	runGitCommand(t, clonePath, "push", "origin", "main")
}

func TestBumpRefusesAWorktreeBehindItsUpstream(t *testing.T) {
	t.Parallel()
	const branchName = "release"

	repository := bumpTestRepository(t)
	require.NoError(t, repository.runTimber(t, "create", at(testRepoName, branchName)).err)
	pushCommitToOriginMain(t, repository.remotePath)

	result := repository.runTimber(t, "bump", "--patch", at(testRepoName, branchName))
	require.ErrorContains(t, result.err, `branch "release" is 1 commit behind origin/main`)
	assert.Empty(t, strings.TrimSpace(runGitCommand(t, repository.remotePath, "tag")))
}
