package timber

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalPruneCancellationKeepsDefaultSelection(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	const branch = "feature/keep"
	require.NoError(t, repository.runTimber(t, "create", at(testRepoName, branch)).err)
	head := runGitCommand(t, repository.worktreePath(branch), "rev-parse", "HEAD")
	command := NewRootCommand(repository.runtime)
	command.SetArgs([]string{"prune", "--prompt", at(testRepoName, "")})
	command.SetIn(strings.NewReader("\x03"))
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)

	err := command.Execute()

	assert.Equal(t, head, runGitCommand(t, repository.worktreePath(branch), "rev-parse", "HEAD"))
	assert.Equal(t, head, runGitCommand(t, repository.barePath, "rev-parse", "refs/heads/"+branch))
	require.Error(t, err)
}
