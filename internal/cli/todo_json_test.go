package cli

import (
	json "encoding/json/v2"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTodoFile(t *testing.T, path, contents string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
}

type todoJSONSummary struct {
	Incomplete int `json:"incomplete"`
	Complete   int `json:"complete"`
}
type todoJSONOutput struct {
	Summary todoJSONSummary `json:"summary"`
	Todos   []struct {
		ID        int    `json:"id"`
		Title     string `json:"title"`
		DependsOn *int   `json:"depends_on"`
		Complete  bool   `json:"complete"`
	} `json:"todos"`
}

func TestTodoJSONCommandEmitsFileAsJSON(t *testing.T) {
	t.Parallel()

	const branchName = "feature/todo-json"
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	worktreePath := testRepository.worktreePath(branchName)
	writeTodoFile(t, todoPathForWorktree(t, worktreePath), strings.Join([]string{
		"- [ ] Top task",
		"  - [x] Child task",
		"- [ ] Other",
	}, "\n")+"\n")

	// EDITOR=false would fail if the editor were launched.
	runtime := withTestEnvironment(testRepository.runtime, "EDITOR=false")
	result := runTimberFromWithRuntime(t, runtime, worktreePath, "todo", "--json")
	require.NoError(t, result.err, result.stderr)

	var output todoJSONOutput
	require.NoError(t, json.Unmarshal([]byte(result.stdout), &output))
	assert.Equal(t, todoJSONSummary{Incomplete: 2, Complete: 1}, output.Summary)
	require.Len(t, output.Todos, 3)
	assert.Equal(t, "Top task", output.Todos[0].Title)
	assert.False(t, output.Todos[0].Complete)
	assert.Nil(t, output.Todos[0].DependsOn)
	require.NotNil(t, output.Todos[1].DependsOn)
	assert.Equal(t, 1, *output.Todos[1].DependsOn)
	assert.True(t, output.Todos[1].Complete)
	assert.Nil(t, output.Todos[2].DependsOn)
}

func TestTodoJSONCommandTreatsMissingFileAsEmpty(t *testing.T) {
	t.Parallel()

	const branchName = "feature/todo-json-missing"
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	worktreePath := testRepository.worktreePath(branchName)
	runtime := withTestEnvironment(testRepository.runtime, "EDITOR=false")
	result := runTimberFromWithRuntime(t, runtime, worktreePath, "todo", "--json")
	require.NoError(t, result.err, result.stderr)

	var output todoJSONOutput
	require.NoError(t, json.Unmarshal([]byte(result.stdout), &output))
	assert.Equal(t, todoJSONSummary{}, output.Summary)
	assert.Empty(t, output.Todos)
}

func TestTodoJSONRejectsPathCombination(t *testing.T) {
	t.Parallel()

	testRepository := newTestRepository(t)
	runtime := withTestEnvironment(testRepository.runtime, "EDITOR=false")

	result := runTimberFromWithRuntime(t, runtime, testRepository.home, "todo", "--path", "--json")
	require.Error(t, result.err)
}
