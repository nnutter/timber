package timber

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestGitHubTokenPrefersEnvironmentOverEverything(t *testing.T) {
	runtime := testRuntime(t)
	runtime.Environment = replaceTestEnvironment(runtime.Environment, "GH_TOKEN=from-gh-token", "GITHUB_TOKEN=from-github-token")
	runtime.GitHubKeyringLookup = keyringReturning(t, "github-keyring", nil)

	token, err := runtime.githubToken()
	require.NoError(t, err)
	assert.Equal(t, "from-gh-token", token)

	runtime.Environment = replaceTestEnvironment(runtime.Environment, "GH_TOKEN=")
	token, err = runtime.githubToken()
	require.NoError(t, err)
	assert.Equal(t, "from-github-token", token)
}

func TestGitHubTokenReadsGitHubCLIKeyring(t *testing.T) {
	for _, test := range []struct {
		name     string
		hosts    string
		lookup   func(t *testing.T, service, user string) (string, error)
		expected string
	}{
		{
			name:  "keyring entry for the active user",
			hosts: "github.com:\n    users:\n        nnutter:\n    user: nnutter\n",
			lookup: func(t *testing.T, service, user string) (string, error) {
				t.Helper()
				assert.Equal(t, "gh:github.com", service)
				if user == "nnutter" {
					return "from-user-keyring", nil
				}
				return "", keyring.ErrNotFound
			},
			expected: "from-user-keyring",
		},
		{
			name:  "older keyring entry without a user",
			hosts: "github.com:\n    user: nnutter\n",
			lookup: func(_ *testing.T, _, user string) (string, error) {
				if user == "" {
					return "from-shared-keyring", nil
				}
				return "", keyring.ErrNotFound
			},
			expected: "from-shared-keyring",
		},
		{
			name: "no hosts file",
			lookup: func(_ *testing.T, _, _ string) (string, error) {
				return "from-shared-keyring", nil
			},
			expected: "from-shared-keyring",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := testRuntime(t)
			runtime.GhConfigDirectory = t.TempDir()
			if test.hosts != "" {
				require.NoError(t, os.WriteFile(filepath.Join(runtime.GhConfigDirectory, ghHostsFileName), []byte(test.hosts), 0o600))
			}
			runtime.GitHubKeyringLookup = func(service, user string) (string, error) {
				return test.lookup(t, service, user)
			}

			token, err := runtime.githubToken()
			require.NoError(t, err)
			assert.Equal(t, test.expected, token)
		})
	}
}

func TestGitHubTokenIgnoresTokensInFiles(t *testing.T) {
	runtime := testRuntime(t)
	runtime.GhConfigDirectory = t.TempDir()
	hosts := "github.com:\n    user: nnutter\n    oauth_token: from-hosts-file\n"
	require.NoError(t, os.WriteFile(filepath.Join(runtime.GhConfigDirectory, ghHostsFileName), []byte(hosts), 0o600))
	runtime.GitHubKeyringLookup = keyringReturning(t, "", keyring.ErrNotFound)

	_, err := runtime.githubToken()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "from-hosts-file")
}

func TestGitHubTokenIgnoresOtherHosts(t *testing.T) {
	runtime := testRuntime(t)
	runtime.GhConfigDirectory = t.TempDir()
	hosts := "github.example.com:\n    user: nnutter\n    oauth_token: enterprise-token\n"
	require.NoError(t, os.WriteFile(filepath.Join(runtime.GhConfigDirectory, ghHostsFileName), []byte(hosts), 0o600))
	runtime.GitHubKeyringLookup = func(_, user string) (string, error) {
		t.Helper()
		assert.Empty(t, user, "another host's user must not select a github.com slot")
		return "", keyring.ErrNotFound
	}

	_, err := runtime.githubToken()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "enterprise-token")
}

func TestGitHubTokenReportsMissingCredential(t *testing.T) {
	runtime := testRuntime(t)
	runtime.GitHubKeyringLookup = keyringReturning(t, "", keyring.ErrNotFound)

	_, err := runtime.githubToken()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GH_TOKEN")
	assert.Contains(t, err.Error(), "gh auth login")
}

func TestGitHubTokenReportsUnusableKeyring(t *testing.T) {
	runtime := testRuntime(t)
	runtime.GhConfigDirectory = t.TempDir()
	hosts := "github.com:\n    user: nnutter\n    oauth_token: from-hosts-file\n"
	require.NoError(t, os.WriteFile(filepath.Join(runtime.GhConfigDirectory, ghHostsFileName), []byte(hosts), 0o600))
	runtime.GitHubKeyringLookup = keyringReturning(t, "", errors.New("dbus is not running"))

	_, err := runtime.githubToken()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keyring")
	assert.NotContains(t, err.Error(), "from-hosts-file")
}

func TestGitHubTokenReportsUnreadableHostsFile(t *testing.T) {
	runtime := testRuntime(t)
	runtime.GhConfigDirectory = t.TempDir()
	runtime.GitHubKeyringLookup = keyringReturning(t, "", keyring.ErrNotFound)
	require.NoError(t, os.WriteFile(filepath.Join(runtime.GhConfigDirectory, ghHostsFileName), []byte("github.com: [\n"), 0o600))

	_, err := runtime.githubToken()
	require.Error(t, err)
	assert.Contains(t, err.Error(), ghHostsFileName)
}

func keyringReturning(t *testing.T, secret string, err error) func(service, user string) (string, error) {
	t.Helper()
	return func(_, _ string) (string, error) {
		return secret, err
	}
}

func TestLookupGitHubKeyringReturnsResult(t *testing.T) {
	// The system keyring is unavailable in CI, so this only asserts that a
	// lookup settles instead of hanging.
	done := make(chan error, 1)
	go func() {
		_, err := lookupGitHubKeyring("gh:timber-test-does-not-exist", "")
		done <- err
	}()
	select {
	case err := <-done:
		// The error depends on the host keyring. Only settling matters here.
		_ = err
	case <-time.After(keyringTimeout + time.Second):
		require.Fail(t, "keyring lookup did not settle")
	}
}
