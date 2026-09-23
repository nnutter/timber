package timber

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func installFakeEditor(t *testing.T, logPath string) string {
	t.Helper()

	dir := resolvedTempDir(t)
	scriptPath := filepath.Join(dir, "fake-editor")
	script := `#!/bin/sh
log_path="` + logPath + `"
last=""
for arg in "$@"; do
  last="$arg"
  printf '%s\n' "$arg" >> "$log_path"
done
printf 'edited\n' >> "$last"
`
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
	return scriptPath
}

func todoPathForWorktree(t *testing.T, worktreePath string) string {
	t.Helper()

	gitDir := strings.TrimSpace(runGitCommand(t, worktreePath, "rev-parse", "--absolute-git-dir"))
	return filepath.Join(gitDir, "TODO.md")
}

func TestTodoOpensWorktreeSpecificFile(t *testing.T) {
	t.Parallel()

	const branchName = "feature/todo"
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	worktreePath := testRepository.worktreePath(branchName)
	expectedTodoPath := todoPathForWorktree(t, worktreePath)

	logPath := filepath.Join(resolvedTempDir(t), "editor.log")
	editor := installFakeEditor(t, logPath)
	runtime := withTestEnvironment(testRepository.runtime, "EDITOR="+editor)

	result := runTimberFromWithRuntime(t, runtime, worktreePath, "todo")
	require.NoError(t, result.err, result.stderr)

	contents, err := os.ReadFile(expectedTodoPath)
	require.NoError(t, err)
	assert.Contains(t, string(contents), "edited\n")

	logContents, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(logContents), expectedTodoPath)

	// An extra file in the worktree admin dir must not break Git.
	runGitCommand(t, worktreePath, "status")
	runGitCommand(t, worktreePath, "worktree", "list")
}

func TestTodoUsesSeparateFilesPerWorktree(t *testing.T) {
	t.Parallel()

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/todo-a")).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/todo-b")).err)

	pathA := testRepository.worktreePath("feature/todo-a")
	pathB := testRepository.worktreePath("feature/todo-b")
	todoA := todoPathForWorktree(t, pathA)
	todoB := todoPathForWorktree(t, pathB)
	require.NotEqual(t, todoA, todoB)

	logPath := filepath.Join(resolvedTempDir(t), "editor.log")
	editor := installFakeEditor(t, logPath)

	result := runTimberFromWithRuntime(t, withTestEnvironment(testRepository.runtime, "EDITOR="+editor), pathA, "todo")
	require.NoError(t, result.err, result.stderr)
	result = runTimberFromWithRuntime(t, withTestEnvironment(testRepository.runtime, "EDITOR="+editor), pathB, "todo")
	require.NoError(t, result.err, result.stderr)

	testRepository.assertPathPresent(t, todoA)
	testRepository.assertPathPresent(t, todoB)
}

func TestTodoPassesEditorArgs(t *testing.T) {
	t.Parallel()

	const branchName = "feature/todo-args"
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	worktreePath := testRepository.worktreePath(branchName)
	expectedTodoPath := todoPathForWorktree(t, worktreePath)

	logPath := filepath.Join(resolvedTempDir(t), "editor.log")
	editor := installFakeEditor(t, logPath)
	runtime := withTestEnvironment(testRepository.runtime, "EDITOR="+editor+" --wait")

	result := runTimberFromWithRuntime(t, runtime, worktreePath, "todo")
	require.NoError(t, result.err, result.stderr)

	logContents, err := os.ReadFile(logPath)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(logContents)), "\n")
	require.Equal(t, []string{"--wait", expectedTodoPath}, lines)
}

func TestTodoFailsWithoutEditorOrFallback(t *testing.T) {
	t.Parallel()

	const branchName = "feature/todo-no-editor"
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	emptyPathDir := resolvedTempDir(t)
	runtime := withTestEnvironment(testRepository.runtime, "EDITOR=", "PATH="+emptyPathDir)

	result := runTimberFromWithRuntime(t, runtime, testRepository.worktreePath(branchName), "todo")
	require.ErrorContains(t, result.err, "EDITOR is not set")
}

func TestTodoFailsOutsideWorktree(t *testing.T) {
	t.Parallel()

	testRepository := newTestRepository(t)
	runtime := withTestEnvironment(testRepository.runtime, "EDITOR=true")

	result := runTimberFromWithRuntime(t, runtime, testRepository.home, "todo")
	require.Error(t, result.err)
}

func TestTodoFailsInBareRepository(t *testing.T) {
	t.Parallel()

	testRepository := newTestRepository(t)
	runtime := withTestEnvironment(testRepository.runtime, "EDITOR=true")

	result := runTimberFromWithRuntime(t, runtime, testRepository.barePath, "todo")
	require.ErrorContains(t, result.err, "not inside a worktree")
}
