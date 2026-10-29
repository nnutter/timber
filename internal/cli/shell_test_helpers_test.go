package cli

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// shellTestCommand isolates subprocesses from the user's shell and Git settings.
func shellTestCommand(t *testing.T, name string, args ...string) *exec.Cmd {
	t.Helper()
	runtime := testRuntime(t)
	command := exec.CommandContext(t.Context(), name, args...)
	command.Dir = runtime.HomeDirectory
	command.Env = shellTestEnvironment(runtime.Environment,
		"TMPDIR="+runtime.HomeDirectory,
		"ZDOTDIR="+runtime.HomeDirectory,
	)
	return command
}

func shellTestEnvironment(environment []string, replacements ...string) []string {
	keys := make(map[string]struct{}, len(replacements))
	for _, replacement := range replacements {
		key, _, _ := strings.Cut(replacement, "=")
		keys[key] = struct{}{}
	}
	updated := make([]string, 0, len(environment)+len(replacements))
	for _, value := range environment {
		key, _, _ := strings.Cut(value, "=")
		if _, replace := keys[key]; !replace {
			updated = append(updated, value)
		}
	}
	return append(updated, replacements...)
}

func shellCanonicalPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return resolved
}

func shellSkipIfNoPty(t *testing.T) {
	t.Helper()
	if err := shellTestCommand(t, "python3", "-c", "import pty; pty.openpty()").Run(); err != nil {
		t.Skip("pty devices are not available")
	}
}
