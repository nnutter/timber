package timber

import (
	"encoding/json/v2"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListSortValue(t *testing.T) {
	t.Parallel()
	command := NewListCommand(Runtime{})
	flag := command.Flags().Lookup("sort")
	require.NotNil(t, flag)
	assert.Equal(t, "recency", flag.DefValue)
	assert.Equal(t, "sort", flag.Value.Type())
	for _, value := range []string{"recency", "worktree", "repo"} {
		require.NoError(t, flag.Value.Set(value))
		assert.Equal(t, value, flag.Value.String())
	}
	for _, value := range []string{"", "unknown", "REPO"} {
		require.ErrorContains(t, flag.Value.Set(value), "invalid sort mode")
		assert.Equal(t, "repo", flag.Value.String())
	}
}

func TestListSortCompletion(t *testing.T) {
	t.Parallel()
	command := NewListCommand(Runtime{})
	complete, found := command.GetFlagCompletionFunc("sort")
	require.True(t, found)
	for _, tc := range []struct {
		prefix string
		want   []string
	}{
		{"", []string{"recency", "repo", "worktree"}},
		{"re", []string{"recency", "repo"}},
		{"rec", []string{"recency"}},
		{"w", []string{"worktree"}},
		{"unknown", nil},
	} {
		values, directive := complete(command, nil, tc.prefix)
		assert.Equal(t, tc.want, values)
		assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	}
}

func TestListSortModes(t *testing.T) {
	t.Parallel()
	fixture := newTestRepository(t)
	registerAdditionalRepo(t, fixture, "aaa")
	for _, name := range []string{"z@aaa", "a@aaa", "a@" + testRepoName} {
		require.NoError(t, fixture.runTimber(t, "create", name).err)
	}
	for _, tc := range []struct {
		mode string
		want []string
	}{
		{"repo", []string{"a@aaa", "z@aaa", "a@" + testRepoName}},
		{"worktree", []string{"a@aaa", "a@" + testRepoName, "z@aaa"}},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			result := fixture.runTimber(t, "list", "--json", "--sort", tc.mode)
			require.NoError(t, result.err, result.stderr)
			var records []listJSONWorktree
			require.NoError(t, json.Unmarshal([]byte(result.stdout), &records))
			var names []string
			for _, record := range records {
				names = append(names, record.Name+"@"+record.Repo)
			}
			assert.Equal(t, tc.want, names)

			table := fixture.runTimber(t, "list", "--sort", tc.mode)
			require.NoError(t, table.err, table.stderr)
			var tableNames []string
			for line := range strings.SplitSeq(table.stdout, "\n") {
				cells := strings.Split(line, "│")
				if len(cells) > 3 && !strings.Contains(cells[1], "Name") {
					tableNames = append(tableNames, strings.TrimSpace(cells[1])+"@"+strings.TrimSpace(cells[2]))
				}
			}
			assert.Equal(t, tc.want, tableNames)
		})
	}
	result := fixture.runTimber(t, "list", "--sort", "unknown")
	require.ErrorContains(t, result.err, "invalid sort mode")
}

func TestListRecencyOrdering(t *testing.T) {
	t.Parallel()
	worktrees := []managedWorktree{
		{Repo: "b", Name: "a", CommitTime: time.Unix(100, 0)},
		{Repo: "a", Name: "z", CommitTime: time.Unix(100, 0)},
		{Repo: "a", Name: "a", CommitTime: time.Unix(100, 0)},
		{Name: "unborn"},
		{Name: "newest", CommitTime: time.Unix(200, 0)},
		{Name: "oldest", CommitTime: time.Unix(-100, 0)},
	}
	listSortRecency.sort(worktrees)
	var names []string
	for _, worktree := range worktrees {
		names = append(names, worktree.Name+"@"+worktree.Repo)
	}
	assert.Equal(t, []string{"newest@", "a@a", "z@a", "a@b", "oldest@", "unborn@"}, names)
}

func TestListDefaultsToCommitterRecency(t *testing.T) {
	t.Parallel()
	fixture := newTestRepository(t)
	registerAdditionalRepo(t, fixture, "aaa")
	for _, name := range []string{"older@aaa", "newer@" + testRepoName} {
		require.NoError(t, fixture.runTimber(t, "create", name).err)
	}
	// Deliberately reverse author dates: recency must use committer dates.
	for _, tc := range []struct{ repo, name, author, committer string }{
		{"aaa", "older", "2025-01-01T00:00:00Z", "2020-01-01T00:00:00Z"},
		{testRepoName, "newer", "2010-01-01T00:00:00Z", "2021-01-01T00:00:00Z"},
	} {
		runtime := withTestEnvironment(fixture.runtime,
			"GIT_AUTHOR_DATE="+tc.author, "GIT_COMMITTER_DATE="+tc.committer,
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		_, err := gitOutput(runtime, runtime.managedWorktreePath(tc.repo, tc.name), "commit", "--allow-empty", "-m", tc.name)
		require.NoError(t, err)
	}
	// A missing checkout must not prevent timestamp lookup or listing.
	require.NoError(t, os.RemoveAll(fixture.worktreePath("newer")))
	for _, args := range [][]string{
		{"list", "--json"},
		{"list", "--json", "--sort", "recency"},
	} {
		result := fixture.runTimber(t, args...)
		require.NoError(t, result.err, result.stderr)
		var records []listJSONWorktree
		require.NoError(t, json.Unmarshal([]byte(result.stdout), &records))
		require.Len(t, records, 2)
		assert.Equal(t, "newer", records[0].Name)
		assert.True(t, records[0].StatusError)
		assert.Equal(t, "older", records[1].Name)
	}
	result := fixture.runTimber(t, "list")
	require.NoError(t, result.err, result.stderr)
	assert.Less(t, strings.Index(result.stdout, "newer"), strings.Index(result.stdout, "older"))
}
