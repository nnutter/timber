package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListSortValue(t *testing.T) {
	t.Parallel()
	command := New(Dependencies{})
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
	command := New(Dependencies{})
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
