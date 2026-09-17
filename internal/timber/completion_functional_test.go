package timber

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoQualifierCompletionOffersRegisteredRepos(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	registerAdditionalRepo(t, testRepository, "other")

	for _, args := range [][]string{
		{"create", "@"},
		{"create", ""},
		{"list", "@"},
		{"prune", "@"},
	} {
		stdout := runCompleteWithRuntime(t, testRepository.runtime, args...)
		assert.Contains(t, stdout, at(testRepoName, ""), "args=%v", args)
		assert.Contains(t, stdout, "@other", "args=%v", args)
	}
}

func TestWorktreeCompletionAddsAtWhenNameIsAmbiguous(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)
	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/login")).err)
	require.NoError(t, primary.runTimber(t, "create", at(secondaryName, "feature/login")).err)
	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/unique")).err)

	stdout := runCompleteWithRuntime(t, primary.runtime, "switch", "")
	assert.Contains(t, stdout, at(testRepoName, "feature/login"))
	assert.Contains(t, stdout, at(secondaryName, "feature/login"))
	assert.Contains(t, stdout, "feature/unique")
	assert.NotContains(t, stdout, "feature/unique@")
	assert.NotContains(t, stdout, "feature/login\n")

	prefix := runCompleteWithRuntime(t, primary.runtime, "switch", "feature/l")
	assert.Contains(t, prefix, at(testRepoName, "feature/login"))
	assert.Contains(t, prefix, at(secondaryName, "feature/login"))

	qualified := runCompleteWithRuntime(t, primary.runtime, "switch", "feature/login@")
	assert.Contains(t, qualified, at(testRepoName, "feature/login"))
	assert.Contains(t, qualified, at(secondaryName, "feature/login"))
}
