package timber

import (
	"encoding/json/v2"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListSortValue(t *testing.T) {
	t.Parallel()
	command := NewListCommand(Runtime{})
	flag := command.Flags().Lookup("sort")
	require.NotNil(t, flag)
	assert.Equal(t, "repo", flag.DefValue)
	assert.Equal(t, "sort", flag.Value.Type())
	for _, value := range []string{"worktree", "repo"} {
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
		{"", []string{"repo", "worktree"}},
		{"re", []string{"repo"}},
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
