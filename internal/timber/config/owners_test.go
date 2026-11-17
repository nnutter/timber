package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateStripGitHubOwners(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		current []string
		add     []string
		remove  []string
		want    []string
	}{
		{
			name: "add to empty list",
			add:  []string{"nnutter"},
			want: []string{"nnutter"},
		},
		{
			name:    "adding an existing owner in another case keeps one entry",
			current: []string{"nnutter"},
			add:     []string{"Nnutter"},
			want:    []string{"nnutter"},
		},
		{
			name:    "removal ignores case",
			current: []string{"nnutter", "acme"},
			remove:  []string{"ACME"},
			want:    []string{"nnutter"},
		},
		{
			name:    "removing an unlisted owner changes nothing",
			current: []string{"nnutter"},
			remove:  []string{"other"},
			want:    []string{"nnutter"},
		},
		{
			name:    "owners are sorted case-insensitively",
			current: []string{"zeta"},
			add:     []string{"Alpha"},
			want:    []string{"Alpha", "zeta"},
		},
		{
			name:    "duplicates in the stored list collapse",
			current: []string{"nnutter", "NNutter"},
			want:    []string{"nnutter"},
		},
		{
			name:    "empty entries in the stored list are dropped",
			current: []string{"nnutter", "  "},
			want:    []string{"nnutter"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := UpdateStripGitHubOwners(test.current, test.add, test.remove)
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestUpdateStripGitHubOwnersRejectsInvalidOwners(t *testing.T) {
	t.Parallel()
	for _, owner := range []string{"", "  ", "nnutter/timber", "nn utter"} {
		t.Run(owner, func(t *testing.T) {
			t.Parallel()
			_, err := UpdateStripGitHubOwners(nil, []string{owner}, nil)
			require.Error(t, err)
		})
	}
}
