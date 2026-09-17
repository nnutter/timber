package timber

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalRepoImportRollsBackFailedWorktreeCreation(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	linked := filepath.Join(filepath.Dir(fixture.clonePath), "linked")
	runGitCommand(t, fixture.clonePath, "worktree", "add", "-b", "feature", linked)

	// Git runs post-checkout after creating a worktree. Fail on the second
	// checkout to exercise cleanup of both complete and partial worktrees.
	hooks := resolvedTempDir(t)
	log := filepath.Join(hooks, "checkouts")
	require.NoError(t, os.WriteFile(filepath.Join(hooks, "post-checkout"), []byte(`#!/bin/sh
printf '%s\n' "$PWD" >> "$IMPORT_CHECKOUT_LOG"
[ "$PWD" != "$IMPORT_FAIL_PATH" ]
`), 0o755))
	fixture.runtime = withTestEnvironment(fixture.runtime,
		"GIT_CONFIG_VALUE_0="+hooks,
		"IMPORT_CHECKOUT_LOG="+log,
		"IMPORT_FAIL_PATH="+fixture.managedWorktreePath("project", "feature"),
	)

	result := fixture.importRun(t, "--name", "project")

	require.Error(t, result.err)
	checkouts, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, []string{
		fixture.managedWorktreePath("project", "main"),
		fixture.managedWorktreePath("project", "feature"),
	}, strings.Split(strings.TrimSpace(string(checkouts)), "\n"))
	fixture.assertWorktreeBranchAt(t, fixture.clonePath, "main")
	fixture.assertWorktreeBranchAt(t, linked, "feature")
	assert.NotContains(t, fixture.trashedPaths(t), fixture.clonePath)
	assert.NotContains(t, fixture.trashedPaths(t), linked)
	assert.NoDirExists(t, fixture.barePath("project"))
	assert.NoDirExists(t, fixture.managedWorktreePath("project", "main"))
	assert.NoDirExists(t, fixture.managedWorktreePath("project", "feature"))

	fixture.runtime = withTestEnvironment(fixture.runtime, "GIT_CONFIG_VALUE_0="+os.DevNull)
	retry := fixture.importRun(t, "--name", "project")
	require.NoError(t, retry.err, retry.stderr)
	fixture.assertWorktreeBranchAt(t, fixture.managedWorktreePath("project", "main"), "main")
	fixture.assertWorktreeBranchAt(t, fixture.managedWorktreePath("project", "feature"), "feature")
}
