package timber

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListWorktreesRequiresSelectedRenderer(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name      string
		options   ListOptions
		errorText string
	}{
		{name: "table", options: ListOptions{SortMode: "recency"}, errorText: "list table renderer is not configured"},
		{name: "JSON", options: ListOptions{JSON: true, SortMode: "recency"}, errorText: "list JSON renderer is not configured"},
		{name: "locations", options: ListOptions{LocationsOnly: true, SortMode: "recency"}, errorText: "list locations renderer is not configured"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runtime := testRuntime(t)
			err := runtime.ListWorktrees(t.Context(), io.Discard, testCase.options, ListRenderers{})
			require.ErrorContains(t, err, testCase.errorText)
		})
	}
}

func TestListWorktreesReturnsRendererError(t *testing.T) {
	t.Parallel()
	writeErr := errors.New("write failed")
	for _, testCase := range []struct {
		name      string
		options   ListOptions
		renderers ListRenderers
	}{
		{
			name:    "table",
			options: ListOptions{SortMode: "recency"},
			renderers: ListRenderers{Table: func(io.Writer, []ListWorktreeView, bool, bool, bool, bool) error {
				return writeErr
			}},
		},
		{
			name:    "JSON",
			options: ListOptions{JSON: true, SortMode: "recency"},
			renderers: ListRenderers{JSON: func(io.Writer, []ListJSONWorktree) error {
				return writeErr
			}},
		},
		{
			name:    "locations",
			options: ListOptions{LocationsOnly: true, SortMode: "recency"},
			renderers: ListRenderers{Locations: func(io.Writer, []string) error {
				return writeErr
			}},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runtime := testRuntime(t)
			err := runtime.ListWorktrees(t.Context(), io.Discard, testCase.options, testCase.renderers)
			assert.ErrorIs(t, err, writeErr)
		})
	}
}
