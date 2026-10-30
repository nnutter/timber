package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPullRequestCreateResolvesManagedWorktrees(t *testing.T) {
	runtime := testRuntime(t)

	result := runTimberCommandWithRuntime(t, runtime, "pr", "create", "--title", "Add login", "feature/login@acme/timber")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "acme/timber")
}
