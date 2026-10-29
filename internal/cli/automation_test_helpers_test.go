package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/timber"
)

// The automation tests seed cache files directly. They do not depend on the
// application's atomic file writer to construct valid or malformed fixtures.
func writeIdentityCache(t *testing.T, runtime timber.Runtime, contents string) {
	t.Helper()
	path := filepath.Join(runtime.DataHome, "timber", "github-identity.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
}

func installIdentityGh(t *testing.T, runtime timber.Runtime) timber.Runtime {
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
