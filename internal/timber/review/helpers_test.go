package review

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/testenv"
)

func gitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return gitTestEnv(t, dir, "", args...)
}

func gitTestEnv(t *testing.T, dir, override string, args ...string) string {
	t.Helper()
	cmd := testenv.GitCommand(t, dir, args...)
	if override != "" {
		cmd.Env = append(cmd.Env, override)
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	return strings.TrimSpace(string(out))
}

func testRepo(t *testing.T) Repository {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	gitTest(t, dir, "init", "-b", "main")
	gitTest(t, dir, "config", "user.name", "Test")
	gitTest(t, dir, "config", "user.email", "test@example.com")
	gitTest(t, dir, "commit", "--allow-empty", "-m", "Base")
	gitTest(t, dir, "checkout", "-b", "feature")
	gitTest(t, dir, "branch", "--set-upstream-to=main")
	return Repository{Dir: dir, Environment: append(testenv.GitEnvironment(), "HOME="+dir, "XDG_DATA_HOME=")}
}
