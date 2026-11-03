package timber

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/timber/bump"
)

const bumpTestBranch = "release"

// newBumpTestRepository registers a repository whose runtime carries a
// committer identity, which an annotated tag needs.
func newBumpTestRepository(t *testing.T) testRepository {
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

// gitLine runs git and returns its output without the trailing newline.
func gitLine(t *testing.T, directory string, args ...string) string {
	t.Helper()
	return strings.TrimSpace(runGitCommand(t, directory, args...))
}

func TestBumpTagCreatesAndPushesTheNextTag(t *testing.T) {
	t.Parallel()
	repository := newBumpTestRepository(t)
	runGitCommand(t, repository.barePath, "tag", "--annotate", "v1.2.3", "--message", "v1.2.3")
	worktreePath := createTestWorktree(t, repository.runtime, testRepoName, bumpTestBranch)

	tag, err := repository.runtime.BumpTag(nil, TagBump{
		Raw:  at(testRepoName, bumpTestBranch),
		Kind: bump.Patch,
	})
	require.NoError(t, err)
	assert.Equal(t, "v1.2.4", tag)

	assert.Equal(t, "tag", gitLine(t, repository.barePath, "cat-file", "-t", tag))
	assert.Equal(t, "v1.2.4", gitLine(t, repository.remotePath, "tag", "--list", tag))
	head := gitLine(t, worktreePath, "rev-parse", "HEAD")
	assert.Equal(t, head, gitLine(t, repository.barePath, "rev-parse", tag+"^{commit}"))
}

func TestBumpTagWithoutPushLeavesTheTagLocal(t *testing.T) {
	t.Parallel()
	repository := newBumpTestRepository(t)
	runGitCommand(t, repository.barePath, "tag", "--annotate", "v1.2.3", "--message", "v1.2.3")
	createTestWorktree(t, repository.runtime, testRepoName, bumpTestBranch)

	tag, err := repository.runtime.BumpTag(nil, TagBump{
		Raw:    at(testRepoName, bumpTestBranch),
		Kind:   bump.Minor,
		NoPush: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "v1.3.0", tag)
	assert.Equal(t, tag, gitLine(t, repository.barePath, "tag", "--list", tag))
	assert.Empty(t, gitLine(t, repository.remotePath, "tag", "--list", tag))
}

func TestBumpTagBootstrapsWithoutTags(t *testing.T) {
	t.Parallel()
	repository := newBumpTestRepository(t)
	createTestWorktree(t, repository.runtime, testRepoName, bumpTestBranch)

	tag, err := repository.runtime.BumpTag(nil, TagBump{
		Raw:  at(testRepoName, bumpTestBranch),
		Kind: bump.Patch,
	})
	require.NoError(t, err)
	assert.Equal(t, "v0.0.1", tag)
}

func TestBumpTagPatternSelectsCandidates(t *testing.T) {
	t.Parallel()
	repository := newBumpTestRepository(t)
	runGitCommand(t, repository.barePath, "tag", "--annotate", "v1.2.3", "--message", "v1.2.3")
	runGitCommand(t, repository.barePath, "tag", "--annotate", "v2.0.0", "--message", "v2.0.0")
	createTestWorktree(t, repository.runtime, testRepoName, bumpTestBranch)

	tag, err := repository.runtime.BumpTag(nil, TagBump{
		Raw:     at(testRepoName, bumpTestBranch),
		Kind:    bump.Patch,
		Pattern: "v1.*",
		NoPush:  true,
	})
	require.NoError(t, err)
	assert.Equal(t, "v1.2.4", tag)
}

func TestBumpTagRejectsPatternWithoutMatch(t *testing.T) {
	t.Parallel()
	repository := newBumpTestRepository(t)
	runGitCommand(t, repository.barePath, "tag", "--annotate", "v1.2.3", "--message", "v1.2.3")
	createTestWorktree(t, repository.runtime, testRepoName, bumpTestBranch)

	_, err := repository.runtime.BumpTag(nil, TagBump{
		Raw:     at(testRepoName, bumpTestBranch),
		Kind:    bump.Patch,
		Pattern: "v2.*",
	})
	require.ErrorContains(t, err, `no tags match pattern "v2.*"`)
}

func TestBumpTagDefaultsToTheCurrentWorktree(t *testing.T) {
	t.Parallel()
	repository := newBumpTestRepository(t)
	worktreePath := createTestWorktree(t, repository.runtime, testRepoName, bumpTestBranch)
	runtime := repository.runtime
	runtime.CurrentDirectory = worktreePath

	tag, err := runtime.BumpTag(nil, TagBump{Kind: bump.Major, NoPush: true})
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", tag)
}
