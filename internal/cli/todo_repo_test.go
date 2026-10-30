package cli

import (
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTodoRepoSharedAcrossWorktrees(t *testing.T) {
	t.Parallel()

	repository := newTestRepository(t)
	const a, b = "feature/repo-a", "feature/repo-b"
	require.NoError(t, repository.runTimber(t, "create", at(testRepoName, a)).err)
	require.NoError(t, repository.runTimber(t, "create", at(testRepoName, b)).err)
	pathA, pathB := repository.worktreePath(a), repository.worktreePath(b)
	commonDir := strings.TrimSpace(runGitCommand(t, pathA, "rev-parse", "--path-format=absolute", "--git-common-dir"))
	repoTodo := filepath.Join(commonDir, "TODO.repo.md")

	result := repository.runTimberFrom(t, pathA, "todo", "--repo", "--path")
	require.NoError(t, result.err, result.stderr)
	require.Equal(t, repoTodo, strings.TrimSpace(result.stdout))
	writeTodoFile(t, repoTodo, "- [ ] shared task\n- [x] shared done\n")
	writeTodoFile(t, todoPathForWorktree(t, pathA), "- [ ] local task\n")
	writeTodoFile(t, todoPathForWorktree(t, pathB), "- [x] local done\n- [ ] local pending\n")

	result = repository.runTimberFrom(t, pathB, "todo", "--repo", "--path")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, repoTodo, strings.TrimSpace(result.stdout))
	result = repository.runTimber(t, "todo", at(testRepoName, b), "--repo", "--json")
	require.NoError(t, result.err, result.stderr)
	var output todoJSONOutput
	require.NoError(t, json.Unmarshal([]byte(result.stdout), &output))
	assert.Equal(t, todoJSONSummary{Incomplete: 1, Complete: 1}, output.Summary)
	require.Len(t, output.Todos, 2)
	assert.Equal(t, "shared task", output.Todos[0].Title)
	assert.Equal(t, "shared done", output.Todos[1].Title)

	result = repository.runTimberFrom(t, pathA, "todo", "--json")
	require.NoError(t, result.err, result.stderr)
	require.NoError(t, json.Unmarshal([]byte(result.stdout), &output))
	assert.Equal(t, todoJSONSummary{Incomplete: 1}, output.Summary)
	require.Len(t, output.Todos, 1)
	assert.Equal(t, "local task", output.Todos[0].Title)

	result = repository.runTimber(t, "list", "--json", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)
	var records []listJSONRecord
	require.NoError(t, json.Unmarshal([]byte(result.stdout), &records))
	counts := make(map[string][2]int)
	for _, record := range records {
		counts[record.Name] = [2]int{record.TodoDone, record.TodoTotal}
	}
	require.Contains(t, counts, a)
	require.Contains(t, counts, b)
	assert.Equal(t, [2]int{0, 1}, counts[a])
	assert.Equal(t, [2]int{1, 2}, counts[b])
}

func TestTodoRepoDoesNotCollideWithMainWorktree(t *testing.T) {
	t.Parallel()

	repository := newTestRepository(t)
	main := resolvedTempDir(t)
	runGitCommand(t, main, "init")
	gitDir := strings.TrimSpace(runGitCommand(t, main, "rev-parse", "--absolute-git-dir"))
	localTodo := filepath.Join(gitDir, "TODO.md")
	repoTodo := filepath.Join(gitDir, "TODO.repo.md")
	writeTodoFile(t, localTodo, "- [ ] main worktree task\n")

	result := repository.runTimberFrom(t, main, "todo", "--repo", "--path")
	require.NoError(t, result.err, result.stderr)
	require.Equal(t, repoTodo, strings.TrimSpace(result.stdout))
	writeTodoFile(t, repoTodo, "- [x] repository task\n")
	contents, err := os.ReadFile(localTodo)
	require.NoError(t, err)
	assert.Equal(t, "- [ ] main worktree task\n", string(contents))
	result = repository.runTimberFrom(t, main, "todo", "--json")
	require.NoError(t, result.err, result.stderr)
	var output todoJSONOutput
	require.NoError(t, json.Unmarshal([]byte(result.stdout), &output))
	require.Len(t, output.Todos, 1)
	assert.Equal(t, "main worktree task", output.Todos[0].Title)
}
