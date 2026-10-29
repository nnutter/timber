package timber

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// buildTestTimberBinary builds the current module into a temporary bin
// directory and returns it, so pty completion tests exercise the same
// binary the generated completion delegates to via 'timber __complete'.
func buildTestTimberBinary(t *testing.T) string {
	t.Helper()
	_, callerFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Join(filepath.Dir(callerFile), "..", "..")
	binDir := resolvedTempDir(t)

	goEnv := testCommand(t, "go", "env", "GOMODCACHE", "GOCACHE", "GOPROXY", "GOTOOLCHAIN")
	goEnv.Dir = repoRoot
	goEnv.Env = os.Environ()
	envOutput, err := goEnv.Output()
	require.NoError(t, err)
	envKeys := []string{"GOMODCACHE", "GOCACHE", "GOPROXY", "GOTOOLCHAIN"}

	build := testCommand(t, "go", "build", "-o", filepath.Join(binDir, "timber"), ".")
	build.Dir = repoRoot
	// testCommand isolates HOME, which would empty the module and build
	// caches; keep the real Go toolchain directories so the build stays
	// offline and fast.
	for index, line := range strings.Split(strings.TrimSpace(string(envOutput)), "\n") {
		if index < len(envKeys) && line != "" {
			build.Env = replaceTestEnvironment(build.Env, envKeys[index]+"="+line)
		}
	}
	buildOutput, err := build.CombinedOutput()
	require.NoError(t, err, string(buildOutput))
	return binDir
}
