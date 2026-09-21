package timber

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalCreateUsesExplicitUpstreamCommit(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	release := filepath.Join(resolvedTempDir(t), "release")
	runGitCommand(t, repository.barePath, "worktree", "add", "-b", "release", release, "main")
	require.NoError(t, os.WriteFile(filepath.Join(release, "release.txt"), []byte("release\n"), 0o644))
	runGitCommand(t, release, "add", "release.txt")
	runGitCommand(t, release, "commit", "-m", "release change")
	runGitCommand(t, release, "push", "origin", "release")
	releaseHead := runGitCommand(t, release, "rev-parse", "HEAD")
	require.NotEqual(t, releaseHead, runGitCommand(t, repository.barePath, "rev-parse", "main"))

	result := repository.runTimber(t, "create", "--upstream", "origin/release", at(testRepoName, "hotfix"))

	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, releaseHead, runGitCommand(t, repository.worktreePath("hotfix"), "rev-parse", "HEAD"))
	assert.Equal(t, "origin/release\n", runGitCommand(t, repository.worktreePath("hotfix"), "rev-parse", "--abbrev-ref", "@{upstream}"))
}
