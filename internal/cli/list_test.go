package cli

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListLocationIsOptionalLastColumn(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	const standardName = "feature/standard"
	created := repository.runTimber(t, "create", at(testRepoName, standardName))
	require.NoError(t, created.err, created.stderr)
	customPath := filepath.Join(resolvedTempDir(t), "checkout with spaces")
	runGitCommand(t, repository.barePath, "worktree", "add", "-b", "feature/manual", customPath, "origin/main")
	repository.runtime.GhExecutable = installFakeGh(t)
	paths := map[string]string{
		standardName:           "~/worktrees/repo/feature/standard/repo",
		"checkout with spaces": customPath,
	}

	without := repository.runTimber(t, "ls")
	require.NoError(t, without.err, without.stderr)
	assert.NotContains(t, without.stdout, "Location")
	for _, path := range paths {
		assert.NotContains(t, without.stdout, path)
	}

	for _, flags := range [][]string{{"-L"}, {"--location"}, {"--location", "--pr"}} {
		result := repository.runTimber(t, append([]string{"ls"}, flags...)...)
		require.NoError(t, result.err, result.stderr)
		seen := 0
		for line := range strings.SplitSeq(result.stdout, "\n") {
			if !strings.HasPrefix(line, "│") {
				continue
			}
			columns := strings.Split(strings.Trim(line, "│"), "│")
			name := strings.TrimSpace(columns[0])
			if name == "Name" {
				assert.Equal(t, "Location", strings.TrimSpace(columns[len(columns)-1]))
				if len(flags) == 2 {
					assert.Equal(t, "PR", strings.TrimSpace(columns[len(columns)-2]))
				}
			}
			if path, found := paths[name]; found {
				assert.Equal(t, path, strings.TrimSpace(columns[len(columns)-1]))
				seen++
			}
		}
		assert.Equal(t, len(paths), seen, "flags=%v", flags)
	}
}

func TestListReportsTodoProgress(t *testing.T) {
	t.Parallel()
	const branchName = "feature/todo-progress"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/no-todo")).err)
	writeTodoFile(t, todoPathForWorktree(t, testRepository.worktreePath(branchName)), "- [x] done\n- [ ] pending\n")

	result := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, "Todo")
	assert.Contains(t, result.stdout, "1/2")
}

func TestListSucceedsWhenUpstreamRefIsMissing(t *testing.T) {
	t.Parallel()
	const branchName = "feature/no-upstream-ref"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	runGitCommand(t, testRepository.barePath, "update-ref", "-d", "refs/remotes/origin/main")

	result := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, branchName)
}

func TestListSupportsLocalUpstream(t *testing.T) {
	t.Parallel()
	const branchName = "feature/local-upstream"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	runGitCommand(t, testRepository.barePath, "branch", "--set-upstream-to", "main", branchName)

	result := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, branchName)
}

func TestListSupportsCustomRemoteUpstream(t *testing.T) {
	t.Parallel()
	const branchName = "feature/custom-remote"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	// Add a second remote-like ref namespace via config.
	runGitCommand(t, testRepository.barePath, "remote", "add", "upstream", testRepository.remotePath)
	runGitCommand(t, testRepository.barePath, "fetch", "upstream")
	runGitCommand(t, testRepository.barePath, "branch", "--set-upstream-to", "upstream/main", branchName)

	result := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)
}

func TestListSucceedsWhenBranchHasNoUpstream(t *testing.T) {
	t.Parallel()
	const branchName = "feature/no-upstream"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	runGitCommand(t, testRepository.barePath, "branch", "--unset-upstream", branchName)

	result := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, branchName)
}

func TestListShowsMergedStatus(t *testing.T) {
	t.Parallel()

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/fresh")).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/ahead")).err)
	testRepository.commitFileInWorktree(t, "feature/ahead", "change.txt", "change\n")

	result := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)
	assert.NotContains(t, result.stdout, "Merged")
	assert.Contains(t, result.stdout, "feature/fresh")
	assert.Contains(t, result.stdout, "feature/ahead")
	assert.Contains(t, result.stdout, "merged")
	assert.NotContains(t, result.stdout, "unmerged")
}

func TestListPullRequests(t *testing.T) {
	t.Parallel()

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/with-pr")).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/without-pr")).err)
	testRepository.runtime.GhExecutable = installFakeGh(t)
	testRepository.runtime = withTestEnvironment(testRepository.runtime,
		`FAKE_GH_PR_JSON=[{"number":42,"headRefName":"feature/with-pr","state":"OPEN","statusCheckRollup":[{"conclusion":"SUCCESS","status":"COMPLETED"}]}]`,
	)

	result := testRepository.runTimber(t, "list", "--pr", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, "PR")
	assert.Contains(t, result.stdout, "#42 ✓")

	withoutPR := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.NoError(t, withoutPR.err, withoutPR.stderr)
	assert.NotContains(t, withoutPR.stdout, "#42")
}

func TestListPullRequestsFailsWhenGhFails(t *testing.T) {
	t.Parallel()

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/pr-fail")).err)
	testRepository.runtime.GhExecutable = installFakeGh(t)
	testRepository.runtime = withTestEnvironment(testRepository.runtime, "FAKE_GH_PR_JSON=not-json")

	result := testRepository.runTimber(t, "list", "--pr", at(testRepoName, ""))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "decode gh pull request list")
}

func TestListAutoDetectsRepoFromManagedWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/auto-list"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	result := testRepository.runTimberFrom(t, testRepository.worktreePath(branchName), "list")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, "Name")
	assert.Contains(t, result.stdout, "Repo")
	assert.Less(t, strings.Index(result.stdout, "Name"), strings.Index(result.stdout, "Repo"))
	assert.Contains(t, result.stdout, testRepoName)
	assert.Contains(t, result.stdout, branchName)
}

func TestListReportsDirtyWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/dirty-list"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.writeFileInWorktree(t, branchName, "dirty.txt", "dirty\n")

	result := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, branchName)
	assert.Contains(t, result.stdout, "dirty")
}

func TestListOutsideManagedWorktreeListsAllRepos(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)
	secondaryBare := repoTestBarePath(primary.runtime, secondaryName)

	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/primary")).err)
	require.NoError(t, primary.runTimber(t, "create", at(secondaryName, "feature/secondary")).err)

	result := primary.runTimber(t, "list")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, testRepoName)
	assert.Contains(t, result.stdout, "feature/primary")
	assert.Contains(t, result.stdout, secondaryName)
	assert.Contains(t, result.stdout, "feature/secondary")
	assert.DirExists(t, secondaryBare)
}

func TestListShowsErrorWhenWorktreeStatusFails(t *testing.T) {
	t.Parallel()

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/healthy")).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/broken")).err)
	require.NoError(t, os.RemoveAll(testRepository.worktreePath("feature/broken")))

	result := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, "feature/healthy")
	assert.Contains(t, result.stdout, "feature/broken")
	assert.Contains(t, result.stdout, "error")
}

func TestListJSONOutputsWorktrees(t *testing.T) {
	t.Parallel()

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/healthy")).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/broken")).err)
	writeTodoFile(t, todoPathForWorktree(t, testRepository.worktreePath("feature/healthy")), "- [x] done\n- [ ] pending\n")
	require.NoError(t, os.RemoveAll(testRepository.worktreePath("feature/broken")))

	result := testRepository.runTimber(t, "list", "--json", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)

	var records []listJSONRecord
	require.NoError(t, json.Unmarshal([]byte(result.stdout), &records))
	require.Len(t, records, 2)

	byName := make(map[string]listJSONRecord, len(records))
	for _, record := range records {
		byName[record.Name] = record
	}

	healthy, found := byName["feature/healthy"]
	require.True(t, found, "expected feature/healthy in %s", result.stdout)
	assert.Equal(t, testRepoName, healthy.Repo)
	assert.Equal(t, testRepository.worktreePath("feature/healthy"), healthy.Path)
	assert.Equal(t, "origin/main", healthy.Upstream)
	assert.NotEmpty(t, healthy.Commit)
	assert.True(t, healthy.Clean)
	assert.False(t, healthy.StatusError)
	assert.Equal(t, 1, healthy.TodoDone)
	assert.Equal(t, 2, healthy.TodoTotal)

	broken, found := byName["feature/broken"]
	require.True(t, found, "expected feature/broken in %s", result.stdout)
	assert.True(t, broken.StatusError)
	assert.NotEmpty(t, broken.Commit)
}

func TestListInsideManagedWorktreeListsAllRepos(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)

	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/primary")).err)
	require.NoError(t, primary.runTimber(t, "create", at(secondaryName, "feature/secondary")).err)

	result := primary.runTimberFrom(t, primary.worktreePath("feature/primary"), "list")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, "feature/primary")
	assert.Contains(t, result.stdout, "feature/secondary")
	assert.Contains(t, result.stdout, secondaryName)
}
