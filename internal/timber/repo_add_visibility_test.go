package timber

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGitHubShortForm(t *testing.T) {
	t.Parallel()

	for input, want := range map[string]string{
		"nnutter/timber":      "nnutter/timber",
		"nnutter/timber.git":  "nnutter/timber",
		"  nnutter/timber  ":  "nnutter/timber",
		"owner/repo-name_2.x": "owner/repo-name_2.x",
	} {
		assert.Equal(t, want, mustParseGitHubShortForm(t, input))
	}

	for _, input := range []string{
		"",
		"just-a-name",
		"owner/repo/extra",
		"owner/",
		"/owner/repo",
		"./owner/repo",
		"~/owner/repo",
		"https://github.com/owner/repo",
		"git@github.com:owner/repo",
		"github.com:owner/repo",
		"owner/repo.git/.git",
		".git",
		"owner/.git",
		"host:path",
	} {
		_, ok := parseGitHubShortForm(input)
		assert.False(t, ok, "input %q", input)
	}
}

func mustParseGitHubShortForm(t *testing.T, input string) string {
	t.Helper()

	short, ok := parseGitHubShortForm(input)
	require.True(t, ok, "input %q", input)
	return short
}

func TestResolveRemoteURLStripsGitSuffixFromShortForm(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "git@github.com:nnutter/timber", mustResolveRemoteURL(t, "nnutter/timber.git"))
	assert.Equal(t, "https://example.com/r.git", mustResolveRemoteURL(t, "https://example.com/r.git"))
	assert.Equal(t, "git@github.com:nnutter/timber.git", mustResolveRemoteURL(t, "git@github.com:nnutter/timber.git"))
}
