package timber

import (
	"context"
	"net/http"
	"net/http/httptest"
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

	assert.Equal(t, "https://github.com/nnutter/timber", mustResolveRemoteURL(t, "nnutter/timber.git"))
	assert.Equal(t, "https://example.com/r.git", mustResolveRemoteURL(t, "https://example.com/r.git"))
	assert.Equal(t, "git@github.com:nnutter/timber.git", mustResolveRemoteURL(t, "git@github.com:nnutter/timber.git"))
}

func TestResolveAddRemoteURLUsesSSHForPrivateRepos(t *testing.T) {
	t.Parallel()

	fixtures := map[string]struct {
		status   int
		input    string
		want     string
		requests int
	}{
		"public uses https":                {http.StatusOK, "nnutter/timber", "https://github.com/nnutter/timber", 1},
		"private uses ssh without suffix":  {http.StatusNotFound, "nnutter/go-template", "git@github.com:nnutter/go-template", 1},
		"public strips git suffix":         {http.StatusOK, "nnutter/timber.git", "https://github.com/nnutter/timber", 1},
		"private strips git suffix":        {http.StatusNotFound, "nnutter/timber.git", "git@github.com:nnutter/timber", 1},
		"server error falls back to https": {http.StatusInternalServerError, "nnutter/timber", "https://github.com/nnutter/timber", 1},
	}

	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var requests int
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				requests++
				writer.WriteHeader(fixture.status)
			}))
			t.Cleanup(server.Close)

			runtime := testRuntime(t)
			runtime.GitHubAPIBaseURL = server.URL
			options := &repoAddCommandOptions{runtime: runtime}

			remoteURL, err := options.resolveAddRemoteURL(context.Background(), fixture.input)
			require.NoError(t, err)
			assert.Equal(t, fixture.want, remoteURL)
			assert.Equal(t, fixture.requests, requests)
		})
	}
}

func TestResolveAddRemoteURLSkipsCheckForExplicitURLs(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		"https://github.com/nnutter/timber",
		"https://github.com/nnutter/timber.git",
		"git@github.com:nnutter/timber",
		"git@github.com:nnutter/timber.git",
		"/local/repo.git",
		"owner/repo/extra",
	} {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			var requests int
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				requests++
				writer.WriteHeader(http.StatusNotFound)
			}))
			t.Cleanup(server.Close)

			runtime := testRuntime(t)
			runtime.GitHubAPIBaseURL = server.URL
			options := &repoAddCommandOptions{runtime: runtime}

			remoteURL, err := options.resolveAddRemoteURL(context.Background(), input)
			require.NoError(t, err)
			assert.Equal(t, mustResolveRemoteURL(t, input), remoteURL)
			assert.Zero(t, requests)
		})
	}
}

func TestResolveAddRemoteURLFallsBackWhenUnreachable(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	server.Close()

	runtime := testRuntime(t)
	runtime.GitHubAPIBaseURL = server.URL
	options := &repoAddCommandOptions{runtime: runtime}

	remoteURL, err := options.resolveAddRemoteURL(context.Background(), "nnutter/timber")
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/nnutter/timber", remoteURL)
}
