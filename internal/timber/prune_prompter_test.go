package timber

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPruneWithPromptRequiresPrompter(t *testing.T) {
	t.Parallel()
	runtime := testRuntime(t)
	err := runtime.PruneWorktrees(nil, io.Discard, "", true, true, nil, PruneReporters{})
	require.ErrorContains(t, err, "prune prompt requires a worktree prompter")
}
