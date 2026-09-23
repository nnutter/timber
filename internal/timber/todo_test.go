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

func TestTodoPathPrintsPathWithoutEditor(t *testing.T) {
	t.Parallel()

	const branchName = "feature/todo-path"
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	worktreePath := testRepository.worktreePath(branchName)
	expectedTodoPath := todoPathForWorktree(t, worktreePath)

	// EDITOR=false would fail if the editor were launched.
	runtime := withTestEnvironment(testRepository.runtime, "EDITOR=false")

	result := runTimberFromWithRuntime(t, runtime, worktreePath, "todo", "--path")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, expectedTodoPath, strings.TrimSpace(result.stdout))
	testRepository.assertPathPresent(t, expectedTodoPath)
}

func TestTodoInstallSkillWritesEmbeddedContent(t *testing.T) {
	t.Parallel()

	testRepository := newTestRepository(t)
	runtime := testRuntimeForHome(resolvedTempDir(t), testRepository.home)
	runtime.TodoSkillContent = "# timber-todo test skill\n"

	result := runTimberFromWithRuntime(t, runtime, testRepository.home, "todo", "--install-skill")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "installed timber-todo skill")

	destination := filepath.Join(runtime.HomeDirectory, ".agents", "skills", "timber-todo", "SKILL.md")
	contents, err := os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, "# timber-todo test skill\n", string(contents))
}

func TestTodoInstallSkillRefusesOverwriteWithoutForce(t *testing.T) {
	t.Parallel()

	testRepository := newTestRepository(t)
	runtime := testRuntimeForHome(resolvedTempDir(t), testRepository.home)
	runtime.TodoSkillContent = "# version one\n"

	result := runTimberFromWithRuntime(t, runtime, testRepository.home, "todo", "--install-skill")
	require.NoError(t, result.err, result.stderr)

	runtime.TodoSkillContent = "# version two\n"
	result = runTimberFromWithRuntime(t, runtime, testRepository.home, "todo", "--install-skill")
	require.ErrorContains(t, result.err, "already exists")

	destination := filepath.Join(runtime.HomeDirectory, ".agents", "skills", "timber-todo", "SKILL.md")
	contents, err := os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, "# version one\n", string(contents))

	result = runTimberFromWithRuntime(t, runtime, testRepository.home, "todo", "--install-skill", "--force")
	require.NoError(t, result.err, result.stderr)
	contents, err = os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, "# version two\n", string(contents))
}

func TestTodoInstallSkillRejectsPathCombination(t *testing.T) {
	t.Parallel()

	testRepository := newTestRepository(t)
	runtime := testRuntimeForHome(resolvedTempDir(t), testRepository.home)
	runtime.TodoSkillContent = "# timber-todo test skill\n"

	result := runTimberFromWithRuntime(t, runtime, testRepository.home, "todo", "--path", "--install-skill")
	require.Error(t, result.err)
}

func TestTodoPathWithWorktreeName(t *testing.T) {
	t.Parallel()

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/todo-a")).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/todo-b")).err)

	expectedTodoPath := todoPathForWorktree(t, testRepository.worktreePath("feature/todo-b"))
	runtime := withTestEnvironment(testRepository.runtime, "EDITOR=false")

	result := runTimberFromWithRuntime(t, runtime, testRepository.worktreePath("feature/todo-a"), "todo", "--path", "feature/todo-b")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, expectedTodoPath, strings.TrimSpace(result.stdout))
}

func TestTodoQualifiedNameFromOutsideWorktree(t *testing.T) {
	t.Parallel()

	const branchName = "feature/todo-qualified"
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	expectedTodoPath := todoPathForWorktree(t, testRepository.worktreePath(branchName))

	logPath := filepath.Join(resolvedTempDir(t), "editor.log")
	editor := installFakeEditor(t, logPath)
	runtime := withTestEnvironment(testRepository.runtime, "EDITOR="+editor)

	result := runTimberFromWithRuntime(t, runtime, testRepository.home, "todo", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)

	logContents, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(logContents), expectedTodoPath)
}

func TestTodoUnknownWorktreeName(t *testing.T) {
	t.Parallel()

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/todo-a")).err)

	runtime := withTestEnvironment(testRepository.runtime, "EDITOR=false")

	result := runTimberFromWithRuntime(t, runtime, testRepository.worktreePath("feature/todo-a"), "todo", "feature/missing")
	require.Error(t, result.err)
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
