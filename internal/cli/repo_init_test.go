package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/timber"
)

// emptyTreeHash is the tree object Git writes for a commit with no entries.
const emptyTreeHash = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// repoInitRuntime supplies the commit identity the empty initial commit needs.
// The shared test runtime deliberately inherits no Git identity.
func repoInitRuntime(t *testing.T) timber.Runtime {
	t.Helper()

	runtime := testRuntime(t)
	runtime.Environment = append(runtime.Environment,
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	return runtime
}

func TestRepoInitCreatesRepositoryWithEmptyInitialCommit(t *testing.T) {
	t.Parallel()

	for _, fixture := range []struct {
		name     string
		args     []string
		repoName string
		want     string
	}{
		{name: "default branch", args: []string{"repo", "init", "demo"}, repoName: "demo", want: "main"},
		{name: "long flag", args: []string{"repo", "init", "demo", "--initial-branch", "master"}, repoName: "demo", want: "master"},
		{name: "short flag", args: []string{"repo", "init", "demo", "-b", "trunk"}, repoName: "demo", want: "trunk"},
		{name: "structured name", args: []string{"repo", "init", "owner/demo"}, repoName: "owner/demo", want: "main"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			runtime := repoInitRuntime(t)
			result := runTimberCommandWithRuntime(t, runtime, fixture.args...)

			require.NoError(t, result.err, result.stderr)
			barePath := repoTestBarePath(runtime, fixture.repoName)
			assert.Contains(t, result.stderr, "initialized repository "+fixture.repoName+" at "+barePath)
			assert.Equal(t, "true\n", runGitCommand(t, barePath, "rev-parse", "--is-bare-repository"))
			assert.Equal(t, fixture.want+"\n", runGitCommand(t, barePath, "symbolic-ref", "--short", "HEAD"))
			assert.Equal(t, "1\n", runGitCommand(t, barePath, "rev-list", "--count", "refs/heads/"+fixture.want))
			assert.Equal(t, emptyTreeHash+"\n", runGitCommand(t, barePath, "rev-parse", "refs/heads/"+fixture.want+"^{tree}"))
			assert.Empty(t, runGitCommand(t, barePath, "remote"))
		})
	}
}

func TestRepoInitRegistersRepository(t *testing.T) {
	t.Parallel()

	runtime := repoInitRuntime(t)
	require.NoError(t, runTimberCommandWithRuntime(t, runtime, "repo", "init", "demo").err)

	result := runTimberCommandWithRuntime(t, runtime, "repo", "list", "--quiet")

	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, "demo\n", result.stdout)
}

func TestRepoInitRejectsExistingRepository(t *testing.T) {
	t.Parallel()

	runtime := repoInitRuntime(t)
	require.NoError(t, runTimberCommandWithRuntime(t, runtime, "repo", "init", "demo").err)

	result := runTimberCommandWithRuntime(t, runtime, "repo", "init", "demo")

	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "already exists")
}

func TestRepoInitRejectsInvalidInitialBranch(t *testing.T) {
	t.Parallel()

	runtime := repoInitRuntime(t)
	result := runTimberCommandWithRuntime(t, runtime, "repo", "init", "demo", "-b", "bad branch")

	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "invalid initial branch")
	assert.NoDirExists(t, filepath.Dir(repoTestBarePath(runtime, "demo")))
}

func TestRepoInitDiscardsRepositoryWithoutCommitIdentity(t *testing.T) {
	t.Parallel()

	for _, repoName := range []string{"demo", "owner/demo"} {
		t.Run(repoName, func(t *testing.T) {
			t.Parallel()

			runtime := testRuntime(t)
			runtime.Environment = requireGitIdentity(runtime.Environment)
			result := runTimberCommandWithRuntime(t, runtime, "repo", "init", repoName)
			require.Error(t, result.err)
			groupDirectory, _, _ := strings.Cut(repoName, "/")
			assert.NoDirExists(t, filepath.Join(runtime.WorktreeRoot, groupDirectory))

			runtime.Environment = append(runtime.Environment,
				"GIT_AUTHOR_NAME=Test User",
				"GIT_AUTHOR_EMAIL=test@example.com",
				"GIT_COMMITTER_NAME=Test User",
				"GIT_COMMITTER_EMAIL=test@example.com",
			)
			retry := runTimberCommandWithRuntime(t, runtime, "repo", "init", repoName)

			require.NoError(t, retry.err, retry.stderr)
		})
	}
}

func TestRepoInitWorktreeLifecycleWithoutOrigin(t *testing.T) {
	t.Parallel()

	runtime := repoInitRuntime(t)
	require.NoError(t, runTimberCommandWithRuntime(t, runtime, "repo", "init", "demo", "-b", "trunk").err)
	initialCommit := strings.TrimSpace(runGitCommand(t, repoTestBarePath(runtime, "demo"), "rev-parse", "refs/heads/trunk"))

	created := runTimberCommandWithRuntime(t, runtime, "create", at("demo", "feature/local"))

	require.NoError(t, created.err, created.stderr)
	worktreePath := repoTestWorktreePath(runtime, "demo", "feature/local")
	assert.Equal(t, "feature/local\n", runGitCommand(t, worktreePath, "branch", "--show-current"))
	assert.Equal(t, initialCommit, strings.TrimSpace(runGitCommand(t, worktreePath, "rev-parse", "HEAD")))

	refused := runTimberCommandWithRuntime(t, runtime, "remove", at("demo", "feature/local"))
	require.Error(t, refused.err)
	assert.Contains(t, refused.err.Error(), "has no upstream branch")

	removed := runTimberCommandWithRuntime(t, runtime, "remove", at("demo", "feature/local"), "--force")
	require.NoError(t, removed.err, removed.stderr)
	assert.NoDirExists(t, worktreePath)
}

func TestRepoInitCompletion(t *testing.T) {
	t.Parallel()

	runtime := testRuntime(t)

	assert.Equal(t, ":4\n", runCompleteWithRuntime(t, runtime, "repo", "init", ""))
	for _, fixture := range []struct {
		args []string
		want []string
	}{
		{args: []string{"repo", "init", "--initial-branch", ""}, want: []string{"main", "master"}},
		{args: []string{"repo", "init", "-b", ""}, want: []string{"main", "master"}},
		{args: []string{"repo", "init", "-b", "mas"}, want: []string{"master"}},
		{args: []string{"repo", "init", "--initial-branch=ma"}, want: []string{"main", "master"}},
	} {
		completion := runCompleteWithRuntime(t, runtime, fixture.args...)
		lines := strings.Split(strings.TrimSpace(completion), "\n")
		require.NotEmpty(t, lines)
		assert.Equal(t, ":4", lines[len(lines)-1], "args=%v", fixture.args)
		assert.ElementsMatch(t, fixture.want, lines[:len(lines)-1], "args=%v", fixture.args)
	}
}
