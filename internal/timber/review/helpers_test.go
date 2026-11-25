package review

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func gitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
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
	return Repository{Dir: dir, Environment: append(os.Environ(), "HOME="+dir, "XDG_DATA_HOME=")}
}
