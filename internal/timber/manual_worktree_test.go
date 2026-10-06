package timber

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManualWorktreeLifecycle(t *testing.T) {
	t.Parallel()
	repo := newTestRepository(t)
	path := filepath.Join(resolvedTempDir(t), "checkout")
	runGitCommand(t, repo.barePath, "worktree", "add", "-b", "feature/manual", path, "origin/main")

	listed := repo.runTimber(t, "list", "--json")
	require.NoError(t, listed.err, listed.stderr)
	var records []listJSONWorktree
	require.NoError(t, json.Unmarshal([]byte(listed.stdout), &records))
	require.Len(t, records, 1)
	assert.Equal(t, "checkout", records[0].Name)
	assert.Equal(t, path, records[0].Path)
	assert.True(t, records[0].Merged)

	for _, name := range []string{"checkout", at(testRepoName, "checkout")} {
		switched := repo.runTimber(t, "switch", name)
		require.NoError(t, switched.err, switched.stderr)
		assert.Equal(t, path, strings.TrimSpace(switched.stdout))
	}
	assert.Contains(t, runCompleteWithRuntime(t, repo.runtime, "switch", "check"), "checkout\n")
	assert.Contains(t, runCompleteWithRuntime(t, repo.runtime, "switch", "checkout@"), at(testRepoName, "checkout"))

	todo := repo.runTimber(t, "todo", "--path", "checkout")
	require.NoError(t, todo.err, todo.stderr)
	gitDir := strings.TrimSpace(runGitCommand(t, path, "rev-parse", "--absolute-git-dir"))
	assert.Equal(t, filepath.Join(gitDir, "TODO.md"), strings.TrimSpace(todo.stdout))

	pruned := repo.runTimber(t, "prune", "--dry-run")
	require.NoError(t, pruned.err, pruned.stderr)
	assert.Contains(t, pruned.stderr, "would prune checkout")

	removed := repo.runTimber(t, "remove", "checkout")
	require.NoError(t, removed.err, removed.stderr)
	assert.NoDirExists(t, path)
	_, err := runGitCommandResult(repo.barePath, "show-ref", "--verify", "refs/heads/feature/manual")
	assert.Error(t, err)
}

func TestManualWorktreeNamesExpandPastConflicts(t *testing.T) {
	t.Parallel()
	repo := newTestRepository(t)
	require.NoError(t, repo.runTimber(t, "create", at(testRepoName, "checkout")).err)
	root := resolvedTempDir(t)
	for _, group := range []string{"one", "two"} {
		path := filepath.Join(root, group, "project", "checkout")
		runGitCommand(t, repo.barePath, "worktree", "add", "-b", "branch-"+group, path, "origin/main")
	}
	listed := repo.runTimber(t, "list", "--json")
	require.NoError(t, listed.err, listed.stderr)
	var records []listJSONWorktree
	require.NoError(t, json.Unmarshal([]byte(listed.stdout), &records))
	var names []string
	for _, record := range records {
		names = append(names, record.Name)
	}
	assert.ElementsMatch(t, []string{"checkout", "one/project/checkout", "two/project/checkout"}, names)
	for _, group := range []string{"one", "two"} {
		switched := repo.runTimber(t, "switch", group+"/project/checkout")
		require.NoError(t, switched.err, switched.stderr)
		assert.Equal(t, filepath.Join(root, group, "project", "checkout"), strings.TrimSpace(switched.stdout))
	}
}
