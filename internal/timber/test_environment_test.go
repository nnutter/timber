package timber

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCommand confines subprocesses to a temporary cwd and HOME. Tests that
// replace PATH must extend command.Env rather than the process environment.
func testCommand(t *testing.T, name string, args ...string) *exec.Cmd {
	t.Helper()
	home := resolvedTempDir(t)
	command := exec.CommandContext(t.Context(), name, args...)
	command.Dir = home
	command.Env = replaceTestEnvironment(testEnvironment(home, filepath.Join(home, ".local", "share")),
		"TMPDIR="+home,
		"ZDOTDIR="+home,
	)
	return command
}

func TestGitTestCommandsIgnoreInheritedRepositoryAndHooks(t *testing.T) {
	// Do not parallelize: deliberately poison this process's environment, but
	// point every override and hook at disposable paths, never the real repo.
	decoy := resolvedTempDir(t)
	runGitCommand(t, decoy, "init")
	runGitCommand(t, decoy, "commit", "--allow-empty", "-m", "decoy")
	decoyHead := runGitCommand(t, decoy, "rev-parse", "HEAD")
	runGitCommand(t, decoy, "branch", "must-stay-missing")

	template := resolvedTempDir(t)
	hookMarker := filepath.Join(template, "hook-ran")
	require.NoError(t, os.MkdirAll(filepath.Join(template, "hooks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(template, "hooks", "post-commit"),
		[]byte("#!/bin/sh\nprintf unexpected > '"+hookMarker+"'\n"), 0o755))

	t.Setenv("GIT_DIR", filepath.Join(decoy, ".git"))
	t.Setenv("GIT_WORK_TREE", decoy)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(decoy, ".git", "index"))
	t.Setenv("GIT_TEMPLATE_DIR", template)

	fixture := resolvedTempDir(t)
	runGitCommand(t, fixture, "init")
	require.NoError(t, os.WriteFile(filepath.Join(fixture, "owned.txt"), []byte("fixture\n"), 0o644))
	runGitCommand(t, fixture, "add", "owned.txt")
	runGitCommand(t, fixture, "commit", "-m", "fixture")

	assert.Equal(t, "owned.txt", strings.TrimSpace(runGitCommand(t, fixture, "ls-files")))
	assert.Equal(t, decoyHead, runGitCommand(t, decoy, "rev-parse", "HEAD"))
	assert.Empty(t, runGitCommand(t, decoy, "status", "--porcelain"))
	assert.NoFileExists(t, hookMarker)
	(testRepository{barePath: filepath.Join(fixture, ".git")}).assertBranchMissing(t, "must-stay-missing")
}
