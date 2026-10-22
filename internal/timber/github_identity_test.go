package timber

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func installIdentityGh(t *testing.T, runtime Runtime) Runtime {
	t.Helper()
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$GH_TEST_LOG"
if [ "${GH_TEST_FAIL:-}" = "1" ]; then
  echo 'GitHub unavailable' >&2
  exit 1
fi
case "$*" in
  'auth status --hostname github.com') exit 0 ;;
  'api --hostname github.com user') printf '%s' '{"login":"alice"}' ;;
  'api --hostname github.com --paginate --slurp user/orgs')
    if [ -n "${GH_TEST_ORGS:-}" ]; then
      printf '%s' "$GH_TEST_ORGS"
    else
      printf '%s' '[[{"login":"first-org"}],[{"login":"second-org"}]]'
    fi ;;
  *) exit 1 ;;
esac
`
	runtime.GhExecutable = filepath.Join(resolvedTempDir(t), "gh")
	require.NoError(t, os.WriteFile(runtime.GhExecutable, []byte(script), 0o700))
	return runtime
}

func TestGitHubIdentityCachesUserAndAllOrganizationPages(t *testing.T) {
	runtime := testRuntime(t)
	runtime.DataHome = filepath.Join(resolvedTempDir(t), "custom-data")
	logPath := filepath.Join(resolvedTempDir(t), "gh.log")
	runtime = installIdentityGh(t, withTestEnvironment(runtime, "GH_TEST_LOG="+logPath))
	identity, err := runtime.githubIdentity(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "alice", identity.Username)
	assert.Equal(t, []string{"first-org", "second-org"}, identity.Organizations)
	path := filepath.Join(runtime.DataHome, "timber", "github-identity.json")
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(contents), `"username":"alice"`)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	before, err := os.ReadFile(logPath)
	require.NoError(t, err)
	runtime = withTestEnvironment(runtime, "GH_TEST_FAIL=1")
	cached, err := runtime.githubIdentity(t.Context())
	require.NoError(t, err)
	assert.Equal(t, identity, cached)
	after, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "fresh cache must not require a GitHub request")
}

func TestGitHubIdentityRefreshesInvalidCachesAndFailsClosed(t *testing.T) {
	for name, contents := range map[string]string{
		"missing":       "",
		"malformed":     "{",
		"expired":       fmt.Sprintf(`{"username":"old-user","organizations":[],"fetched_at":%q}`, time.Now().Add(-25*time.Hour).Format(time.RFC3339Nano)),
		"future":        fmt.Sprintf(`{"username":"old-user","organizations":[],"fetched_at":%q}`, time.Now().Add(time.Hour).Format(time.RFC3339Nano)),
		"invalid login": fmt.Sprintf(`{"username":"","organizations":[],"fetched_at":%q}`, time.Now().Format(time.RFC3339Nano)),
	} {
		t.Run(name, func(t *testing.T) {
			runtime := testRuntime(t)
			logPath := filepath.Join(resolvedTempDir(t), "gh.log")
			runtime = installIdentityGh(t, withTestEnvironment(runtime, "GH_TEST_LOG="+logPath, "GH_TEST_FAIL=1"))
			path := filepath.Join(runtime.DataHome, "timber", "github-identity.json")
			if contents != "" {
				require.NoError(t, writePrivateFile(path, []byte(contents)))
			}
			_, err := runtime.githubIdentity(t.Context())
			require.ErrorContains(t, err, "refresh failed")
			runtime = withTestEnvironment(runtime, "GH_TEST_FAIL=0")
			identity, err := runtime.githubIdentity(t.Context())
			require.NoError(t, err)
			assert.Equal(t, "alice", identity.Username)
			assert.Equal(t, []string{"first-org", "second-org"}, identity.Organizations)
		})
	}
}

func TestGitHubIdentityDoesNotCacheIncompleteOrganizationLookup(t *testing.T) {
	runtime := testRuntime(t)
	runtime = installIdentityGh(t, withTestEnvironment(runtime,
		"GH_TEST_LOG="+filepath.Join(resolvedTempDir(t), "gh.log"), "GH_TEST_ORGS={"))
	_, err := runtime.githubIdentity(t.Context())
	require.ErrorContains(t, err, "decode GitHub organizations")
	_, err = os.Stat(filepath.Join(runtime.DataHome, "timber", "github-identity.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}
