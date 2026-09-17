package timber

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalRuntimeDefaultsDirectoriesUnderHome(t *testing.T) {
	// Capture only; never run Git with an inherited process environment.
	home := resolvedTempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("TIMBER_WORKTREE_ROOT", "")

	runtime, err := RuntimeFromProcess()

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".local", "share"), runtime.DataHome)
	assert.Equal(t, filepath.Join(home, ".config"), runtime.ConfigHome)
	assert.Equal(t, filepath.Join(home, "worktrees"), runtime.WorktreeRoot)
}
