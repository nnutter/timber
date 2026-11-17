package cli

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

func TestConfigCompletionOffersSettingsAndValues(t *testing.T) {
	t.Parallel()
	runtime := testRuntime(t)
	require.NoError(t, runTimberCommandWithRuntime(t, runtime, "config", "append", "strip_github_owners", "nnutter", "acme").err)

	keys := runCompleteWithRuntime(t, runtime, "config", "get", "")
	assert.Contains(t, keys, "auto_mise_trust\t")
	assert.Contains(t, keys, "strip_github_owners\t")
	assert.NotContains(t, keys, "auto_mise_trust_decided")

	values := runCompleteWithRuntime(t, runtime, "config", "set", "auto_mise_trust", "")
	assert.Contains(t, values, "true")
	assert.Contains(t, values, "false")

	owners := runCompleteWithRuntime(t, runtime, "config", "remove", "strip_github_owners", "")
	assert.Contains(t, owners, "nnutter")

	remaining := runCompleteWithRuntime(t, runtime, "config", "remove", "strip_github_owners", "ACME", "")
	assert.Contains(t, remaining, "nnutter")
	assert.NotContains(t, remaining, "acme")
	assert.NotContains(t, runCompleteWithRuntime(t, runtime, "config", "set", "auto_mise_trust", "true", ""), "false")

	// Appending takes values that do not exist yet, so there is nothing to offer.
	assert.NotContains(t, runCompleteWithRuntime(t, runtime, "config", "append", "auto_mise_trust", ""), "true")
}

func TestAliasCompletionExpandsToCommandName(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	for _, testCase := range []struct{ alias, name string }{
		{"rp", "repo"},
		{"hk", "hooks"},
		{"td", "todo"},
	} {
		stdout := runCompleteWithRuntime(t, testRepository.runtime, testCase.alias)
		assert.Contains(t, stdout, testCase.name+"\n", "alias=%s", testCase.alias)
		assert.NotContains(t, stdout, testCase.name+"\t", "alias=%s", testCase.alias)
	}

	nested := runCompleteWithRuntime(t, testRepository.runtime, "rp", "mv")
	assert.Contains(t, nested, "rename\n")
}

func TestAliasCompletionYieldsToCommandNamePrefixes(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	// 'r' is the alias for review, but 'review', 'repo', and 'remove' share
	// the prefix, so completion lists the names instead of expanding.
	stdout := runCompleteWithRuntime(t, testRepository.runtime, "r")
	assert.NotContains(t, stdout, "review\n")
	assert.Contains(t, stdout, "repo\t")

	// 'c' is the alias for create, but 'config' and 'create' share the prefix.
	stdout = runCompleteWithRuntime(t, testRepository.runtime, "c")
	assert.NotContains(t, stdout, "create\n")
	assert.Contains(t, stdout, "create\t")
}
