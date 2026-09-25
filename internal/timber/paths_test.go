package timber

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeRepoNameStripsGitSuffix(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "roam", normalizeRepoName("roam.git"))
	assert.Equal(t, "roam", normalizeRepoName(" roam.git "))
	assert.Equal(t, "roam", normalizeRepoName("roam"))
	assert.Equal(t, "nnutter/timber", normalizeRepoName("nnutter/timber.git"))
	assert.Equal(t, "nnutter/timber", normalizeRepoName("nnutter/timber/"))
	assert.Equal(t, "nnutter/timber", normalizeRepoName("/nnutter/timber/"))
}

func TestRepoShortName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "roam", repoShortName("roam"))
	assert.Equal(t, "timber", repoShortName("nnutter/timber"))
	assert.Equal(t, "c", repoShortName("a/b/c"))
}

func TestValidateRepoName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"roam", "nnutter/timber", "a/b/c"} {
		require.NoError(t, validateRepoName(name), "name %q", name)
	}
	for _, name := range []string{
		"", "roam.git", "nnutter/timber.git", "a//b", "/a/b", "a/b/",
		"a/./b", "a/../b", ".", "..", "a\\b", "a@b", "a:b",
	} {
		require.Error(t, validateRepoName(name), "name %q", name)
	}
}
