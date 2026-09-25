package timber

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoAddMapsGitHubRelativePath(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "https://github.com/nnutter/timber", mustResolveRemoteURL(t, "nnutter/timber"))
	assert.Equal(t, "https://example.com/r.git", mustResolveRemoteURL(t, "https://example.com/r.git"))
	assert.Equal(t, "git@github.com:nnutter/timber.git", mustResolveRemoteURL(t, "git@github.com:nnutter/timber.git"))
}

func TestDefaultRepoNameFromRemote(t *testing.T) {
	t.Parallel()
	name, err := defaultRepoNameFromRemote("https://github.com/nnutter/timber.git")
	require.NoError(t, err)
	assert.Equal(t, "nnutter/timber", name)

	name, err = defaultRepoNameFromRemote("git@github.com:nnutter/timber.git")
	require.NoError(t, err)
	assert.Equal(t, "nnutter/timber", name)

	name, err = defaultRepoNameFromRemote("git@gitlab.com:group/sub/repo.git")
	require.NoError(t, err)
	assert.Equal(t, "group/sub/repo", name)

	name, err = defaultRepoNameFromRemote("https://example.com/r.git")
	require.NoError(t, err)
	assert.Equal(t, "r", name)

	name, err = defaultRepoNameFromRemote("/local/path/roam.git")
	require.NoError(t, err)
	assert.Equal(t, "roam", name)
}

func TestDefaultRepoAliasFromRemote(t *testing.T) {
	t.Parallel()
	for _, origin := range []string{
		"git@github.com:nnutter/agentctl",
		"git@github.com:nnutter/agentctl.git",
		"github.com:nnutter/agentctl.git",
		"https://github.com/nnutter/agentctl",
		"https://github.com/nnutter/agentctl.git/",
		"http://github.com/nnutter/agentctl",
		"git://github.com/nnutter/agentctl.git",
		"ssh://git@github.com/nnutter/agentctl.git",
		"ssh://git@ssh.github.com:443/nnutter/agentctl.git",
		"https://GitHub.com/nnutter/agentctl.git",
	} {
		t.Run(origin, func(t *testing.T) {
			assert.Equal(t, "nnutter/agentctl", defaultRepoAliasFromRemote(origin))
		})
	}
	for _, origin := range []string{
		"", "/local/repo.git", "../repo.git",
		"https://gitlab.com/org/repo.git", "git@gitlab.com:org/repo.git",
		"https://github.com.evil.example/org/repo.git",
		"https://github.com/org", "https://github.com/org/repo/extra",
	} {
		t.Run(origin, func(t *testing.T) {
			assert.Equal(t, origin, defaultRepoAliasFromRemote(origin))
		})
	}
}
