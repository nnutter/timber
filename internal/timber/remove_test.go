package timber

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemoveEmptyParentsStopsAtHome(t *testing.T) {
	t.Parallel()
	homeDirectory := resolvedTempDir(t)

	leafPath := filepath.Join(homeDirectory, "src", "github.com", "nnutter", "repo")
	require.NoError(t, os.MkdirAll(leafPath, 0o755))

	require.NoError(t, removeEmptyParents(leafPath, homeDirectory))

	_, err := os.Stat(leafPath)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(homeDirectory, "src"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(homeDirectory)
	require.NoError(t, err)
}

func TestRemoveEmptyParentsLeavesNonEmptyAncestor(t *testing.T) {
	t.Parallel()
	homeDirectory := resolvedTempDir(t)

	parentPath := filepath.Join(homeDirectory, "src", "github.com", "nnutter")
	leafPath := filepath.Join(parentPath, "repo")
	siblingPath := filepath.Join(parentPath, "other")
	require.NoError(t, os.MkdirAll(leafPath, 0o755))
	require.NoError(t, os.MkdirAll(siblingPath, 0o755))

	require.NoError(t, removeEmptyParents(leafPath, homeDirectory))

	_, err := os.Stat(leafPath)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(siblingPath)
	require.NoError(t, err)
	_, err = os.Stat(parentPath)
	require.NoError(t, err)
}

func TestRemoveEmptyParentsHonorsStopPath(t *testing.T) {
	t.Parallel()
	homeDirectory := resolvedTempDir(t)

	stopPath := filepath.Join(homeDirectory, "worktrees")
	leafPath := filepath.Join(stopPath, "feature", "repo")
	require.NoError(t, os.MkdirAll(leafPath, 0o755))

	require.NoError(t, removeEmptyParents(leafPath, stopPath))

	_, err := os.Stat(leafPath)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(stopPath, "feature"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(stopPath)
	require.NoError(t, err)
}
