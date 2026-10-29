package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/timber"
)

// importFixture builds a bare remote plus a plain clone outside the Timber
// layout, ready for import. Removals go through a fake trash CLI that records
// every path it receives.
type importFixture struct {
	runtime    timber.Runtime
	clonePath  string
	remotePath string
	trashLog   string
}

// installFakeTrash writes a trash CLI that logs and removes the paths it is
// given, emulating a successful move to the system trash.
func installFakeTrash(t *testing.T) (scriptPath string, logPath string) {
	t.Helper()

	binDir := resolvedTempDir(t)
	logPath = filepath.Join(resolvedTempDir(t), "trash.log")
	scriptPath = filepath.Join(binDir, "trash")
	script := fmt.Sprintf(`#!/bin/sh
for arg in "$@"; do
  printf '%%s\n' "$arg" >> %q
  rm -rf "$arg"
done
`, logPath)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
	return scriptPath, logPath
}

func newImportFixture(t *testing.T) importFixture {
	t.Helper()

	runtime := testRuntime(t)
	trashExecutable, trashLog := installFakeTrash(t)
	runtime.TrashExecutable = trashExecutable

	base := resolvedTempDir(t)
	remotePath := filepath.Join(base, "remote.git")
	runGitCommand(t, base, "init", "--bare", remotePath)
	seedImportRemote(t, remotePath)

	clonePath := filepath.Join(base, "project")
	runGitCommand(t, base, "clone", remotePath, clonePath)

	return importFixture{runtime: runtime, clonePath: clonePath, remotePath: remotePath, trashLog: trashLog}
}

func (x importFixture) trashedPaths(t *testing.T) []string {
	t.Helper()
	contents, err := os.ReadFile(x.trashLog)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(contents)), "\n")
}

func (x importFixture) importRun(t *testing.T, args ...string) commandResult {
	t.Helper()
	fullArgs := append([]string{"repo", "import", x.clonePath}, args...)
	return runTimberFromWithRuntime(t, x.runtime, x.runtime.HomeDirectory, fullArgs...)
}

func (x importFixture) barePath(repoName string) string {
	return filepath.Join(x.runtime.WorktreeRoot, filepath.FromSlash(repoName), ".git")
}

func (x importFixture) managedWorktreePath(repoName string, name string) string {
	return filepath.Join(x.runtime.WorktreeRoot, filepath.FromSlash(repoName), name, filepath.Base(repoName))
}

func (x importFixture) assertImportedBareSetup(t *testing.T, repoName string) {
	t.Helper()

	barePath := x.barePath(repoName)
	_, err := os.Stat(barePath)
	require.NoError(t, err)

	// Import must install the same origin tracking setup as repo add.
	fetch := strings.TrimSpace(runGitCommand(t, barePath, "config", "--get", "remote.origin.fetch"))
	assert.Equal(t, "+refs/heads/*:refs/remotes/origin/*", fetch)
	originURL := strings.TrimSpace(runGitCommand(t, barePath, "remote", "get-url", "origin"))
	assert.Equal(t, x.remotePath, originURL)
	originHead := strings.TrimSpace(runGitCommand(t, barePath, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"))
	assert.Equal(t, "origin/main", originHead)
}

func (x importFixture) assertWorktreeBranchAt(t *testing.T, path string, branchName string) {
	t.Helper()

	runGitCommand(t, path, "status")
	branch := strings.TrimSpace(runGitCommand(t, path, "rev-parse", "--abbrev-ref", "HEAD"))
	assert.Equal(t, branchName, branch)
}

func seedImportRemote(t *testing.T, remotePath string) {
	t.Helper()
	seed := filepath.Join(resolvedTempDir(t), "seed")
	runGitCommand(t, filepath.Dir(seed), "clone", remotePath, seed)
	require.NoError(t, os.WriteFile(filepath.Join(seed, "README.md"), []byte("initial\n"), 0o644))
	runGitCommand(t, seed, "add", "README.md")
	runGitCommand(t, seed, "commit", "-m", "initial")
	runGitCommand(t, seed, "branch", "-M", "main")
	runGitCommand(t, seed, "push", "-u", "origin", "main")
	runGitCommand(t, remotePath, "symbolic-ref", "HEAD", "refs/heads/main")
}
