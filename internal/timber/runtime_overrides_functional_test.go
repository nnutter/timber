package timber

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalRuntimeCapturesDirectoryOverrides(t *testing.T) {
	// This tests process-state capture only. Do not execute commands with this
	// runtime: its Environment intentionally reflects the invoking process.
	home := resolvedTempDir(t)
	custom := resolvedTempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(custom, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(custom, "config"))
	t.Setenv("TIMBER_WORKTREE_ROOT", filepath.Join(custom, "worktrees"))

	runtime, err := RuntimeFromProcess()

	require.NoError(t, err)
	assert.Equal(t, home, runtime.HomeDirectory)
	assert.Equal(t, filepath.Join(custom, "data"), runtime.DataHome)
	assert.Equal(t, filepath.Join(custom, "config"), runtime.ConfigHome)
	assert.Equal(t, filepath.Join(custom, "worktrees"), runtime.WorktreeRoot)
}
