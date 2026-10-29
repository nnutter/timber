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
		name       string
		jsonOutput bool
		errorText  string
	}{
		{name: "table", errorText: "list table renderer is not configured"},
		{name: "JSON", jsonOutput: true, errorText: "list JSON renderer is not configured"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runtime := testRuntime(t)
			err := runtime.ListWorktrees(t.Context(), io.Discard, "", false, false, testCase.jsonOutput, "recency", ListRenderers{})
			require.ErrorContains(t, err, testCase.errorText)
		})
	}
}

func TestListWorktreesReturnsRendererError(t *testing.T) {
	t.Parallel()
	writeErr := errors.New("write failed")
	for _, testCase := range []struct {
		name       string
		jsonOutput bool
		renderers  ListRenderers
	}{
		{
			name: "table",
			renderers: ListRenderers{Table: func(io.Writer, []ListWorktreeView, bool, bool) error {
				return writeErr
			}},
		},
		{
			name:       "JSON",
			jsonOutput: true,
			renderers: ListRenderers{JSON: func(io.Writer, []ListJSONWorktree) error {
				return writeErr
			}},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runtime := testRuntime(t)
			err := runtime.ListWorktrees(t.Context(), io.Discard, "", false, false, testCase.jsonOutput, "recency", testCase.renderers)
			assert.ErrorIs(t, err, writeErr)
		})
	}
}
