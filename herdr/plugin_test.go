package herdr

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWritePluginReplacesDestinationAndPreservesExecutables(t *testing.T) {
	t.Parallel()

	destination := filepath.Join(t.TempDir(), "timber")
	stalePath := filepath.Join(destination, "stale")
	require.NoError(t, os.MkdirAll(destination, 0o755))
	require.NoError(t, os.WriteFile(stalePath, []byte("stale\n"), 0o644))

	require.NoError(t, WritePlugin(destination))

	_, err := os.Stat(stalePath)
	require.ErrorIs(t, err, os.ErrNotExist, "stale file still present")

	names, err := EmbeddedFiles()
	require.NoError(t, err)
	require.NotEmpty(t, names)
	for _, name := range names {
		got, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(name)))
		require.NoError(t, err)
		want, err := pluginFiles.ReadFile(name)
		require.NoError(t, err)
		require.Equal(t, string(want), string(got), "%s: installed contents differ from embed", name)
	}

	for _, name := range names {
		if filepath.ToSlash(name) == "herdr-plugin.toml" {
			continue
		}
		info, err := os.Stat(filepath.Join(destination, filepath.FromSlash(name)))
		require.NoError(t, err)
		require.NotZero(t, info.Mode()&0o111, "%s is not executable", name)
	}
}
