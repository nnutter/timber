package herdr

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFunctionalPluginEntrypoint(t *testing.T) {
	t.Parallel()
	script, err := filepath.Abs("test/plugin_test.sh")
	require.NoError(t, err)
	home := t.TempDir()
	command := exec.CommandContext(t.Context(), "/bin/bash", script)
	command.Dir = home
	command.Env = []string{
		"PATH=/usr/bin:/bin",
		"HOME=" + home,
		"TMPDIR=" + home,
	}

	output, err := command.CombinedOutput()

	require.NoError(t, err, string(output))
}
