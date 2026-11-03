package timber

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// withFakeBrowser puts an open stub first on the runtime PATH and returns the
// runtime with the path of the file that records the opened URL.
func withFakeBrowser(t *testing.T, runtime Runtime, succeed bool) (Runtime, string) {
	t.Helper()
	binDir := resolvedTempDir(t)
	logPath := filepath.Join(binDir, "opened")
	script := "#!/bin/sh\nprintf '%s\\n' \"$1\" >> " + fmt.Sprintf("%q", logPath) + "\n"
	if !succeed {
		script = "#!/bin/sh\necho 'no display' >&2\nexit 1\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "open"), []byte(script), 0o755))
	path := binDir + string(os.PathListSeparator) + os.Getenv("PATH")
	return withTestEnvironment(runtime, "PATH="+path), logPath
}

func TestBumpTagCreatesAndPushesTheNextTag(t *testing.T) {
	t.Parallel()
	repository := newBumpTestRepository(t)
	runGitCommand(t, repository.barePath, "tag", "--annotate", "v1.2.3", "--message", "v1.2.3")
	worktreePath := createTestWorktree(t, repository.runtime, testRepoName, bumpTestBranch)

	result, err := repository.runtime.BumpTag(t.Context(), nil, io.Discard, TagBump{
		Raw:  at(testRepoName, bumpTestBranch),
		Kind: bump.Patch,
	})
	require.NoError(t, err)
	assert.Equal(t, "v1.2.4", result.Tag)
	assert.Empty(t, result.ReleaseURL)

	assert.Equal(t, "tag", gitLine(t, repository.barePath, "cat-file", "-t", result.Tag))
	assert.Equal(t, "v1.2.4", gitLine(t, repository.remotePath, "tag", "--list", result.Tag))
	head := gitLine(t, worktreePath, "rev-parse", "HEAD")
	assert.Equal(t, head, gitLine(t, repository.barePath, "rev-parse", result.Tag+"^{commit}"))
}

func TestBumpTagWithoutPushLeavesTheTagLocal(t *testing.T) {
	t.Parallel()
	repository := newBumpTestRepository(t)
	runGitCommand(t, repository.barePath, "tag", "--annotate", "v1.2.3", "--message", "v1.2.3")
	createTestWorktree(t, repository.runtime, testRepoName, bumpTestBranch)

	result, err := repository.runtime.BumpTag(t.Context(), nil, io.Discard, TagBump{
		Raw:    at(testRepoName, bumpTestBranch),
		Kind:   bump.Minor,
		NoPush: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "v1.3.0", result.Tag)
	assert.Equal(t, result.Tag, gitLine(t, repository.barePath, "tag", "--list", result.Tag))
	assert.Empty(t, gitLine(t, repository.remotePath, "tag", "--list", result.Tag))
}

func TestBumpTagBootstrapsWithoutTags(t *testing.T) {
	t.Parallel()
	repository := newBumpTestRepository(t)
	createTestWorktree(t, repository.runtime, testRepoName, bumpTestBranch)

	result, err := repository.runtime.BumpTag(t.Context(), nil, io.Discard, TagBump{
		Raw:  at(testRepoName, bumpTestBranch),
		Kind: bump.Patch,
	})
	require.NoError(t, err)
	assert.Equal(t, "v0.0.1", result.Tag)
}

func TestBumpTagPatternSelectsCandidates(t *testing.T) {
	t.Parallel()
	repository := newBumpTestRepository(t)
	runGitCommand(t, repository.barePath, "tag", "--annotate", "v1.2.3", "--message", "v1.2.3")
	runGitCommand(t, repository.barePath, "tag", "--annotate", "v2.0.0", "--message", "v2.0.0")
	createTestWorktree(t, repository.runtime, testRepoName, bumpTestBranch)

	result, err := repository.runtime.BumpTag(t.Context(), nil, io.Discard, TagBump{
		Raw:     at(testRepoName, bumpTestBranch),
		Kind:    bump.Patch,
		Pattern: "v1.*",
		NoPush:  true,
	})
	require.NoError(t, err)
	assert.Equal(t, "v1.2.4", result.Tag)
}

func TestBumpTagRejectsPatternWithoutMatch(t *testing.T) {
	t.Parallel()
	repository := newBumpTestRepository(t)
	runGitCommand(t, repository.barePath, "tag", "--annotate", "v1.2.3", "--message", "v1.2.3")
	createTestWorktree(t, repository.runtime, testRepoName, bumpTestBranch)

	_, err := repository.runtime.BumpTag(t.Context(), nil, io.Discard, TagBump{
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

	result, err := runtime.BumpTag(t.Context(), nil, io.Discard, TagBump{Kind: bump.Major, NoPush: true})
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", result.Tag)
}

func TestBumpTagCreatesDraftRelease(t *testing.T) {
	t.Parallel()
	const releaseURL = "https://github.com/owner/timber/releases/tag/v0.0.1"
	repository := newBumpTestRepository(t)
	repository.runtime.GhExecutable = installFakeReleaseGh(t, releaseURL)
	createTestWorktree(t, repository.runtime, testRepoName, bumpTestBranch)

	result, err := repository.runtime.BumpTag(t.Context(), nil, io.Discard, TagBump{
		Raw:     at(testRepoName, bumpTestBranch),
		Kind:    bump.Patch,
		Release: true,
		NoOpen:  true,
	})
	require.NoError(t, err)
	assert.Equal(t, "v0.0.1", result.Tag)
	assert.Equal(t, releaseURL, result.ReleaseURL)
}

func TestBumpTagOpensTheDraftRelease(t *testing.T) {
	t.Parallel()
	const releaseURL = "https://github.com/owner/timber/releases/tag/v0.0.1"
	repository := newBumpTestRepository(t)
	repository.runtime.GhExecutable = installFakeReleaseGh(t, releaseURL)
	var logPath string
	repository.runtime, logPath = withFakeBrowser(t, repository.runtime, true)
	createTestWorktree(t, repository.runtime, testRepoName, bumpTestBranch)

	_, err := repository.runtime.BumpTag(t.Context(), nil, io.Discard, TagBump{
		Raw:     at(testRepoName, bumpTestBranch),
		Kind:    bump.Patch,
		Release: true,
	})
	require.NoError(t, err)
	opened, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Equal(t, releaseURL+"\n", string(opened))
}

func TestBumpTagWarnsWhenTheBrowserFails(t *testing.T) {
	t.Parallel()
	repository := newBumpTestRepository(t)
	repository.runtime.GhExecutable = installFakeReleaseGh(t, "https://github.com/owner/timber/releases/tag/v0.0.1")
	repository.runtime, _ = withFakeBrowser(t, repository.runtime, false)
	createTestWorktree(t, repository.runtime, testRepoName, bumpTestBranch)

	var warnings strings.Builder
	result, err := repository.runtime.BumpTag(t.Context(), nil, &warnings, TagBump{
		Raw:     at(testRepoName, bumpTestBranch),
		Kind:    bump.Patch,
		Release: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "v0.0.1", result.Tag)
	assert.Contains(t, warnings.String(), "warning: could not open browser:")
}

func TestBumpTagReportsReleaseFailure(t *testing.T) {
	t.Parallel()
	repository := newBumpTestRepository(t)
	repository.runtime.GhExecutable = installFakeReleaseGh(t, "")
	createTestWorktree(t, repository.runtime, testRepoName, bumpTestBranch)

	_, err := repository.runtime.BumpTag(t.Context(), nil, io.Discard, TagBump{
		Raw:     at(testRepoName, bumpTestBranch),
		Kind:    bump.Patch,
		Release: true,
		NoOpen:  true,
	})
	require.ErrorContains(t, err, "no URL in output")
}
