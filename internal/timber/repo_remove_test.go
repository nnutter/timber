package timber

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoRemoveRefusesWhenWorktreesExist(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/keep")).err)

	result := testRepository.runTimber(t, "repo", "remove", testRepoName)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "still has")
}
