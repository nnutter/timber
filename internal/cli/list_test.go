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

func TestListLocationsOnly(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)

	empty := repository.runTimber(t, "ls", "--locations-only")
	require.NoError(t, empty.err, empty.stderr)
	assert.Empty(t, empty.stdout)

	registerAdditionalRepo(t, repository, "aaa")
	const standardName = "feature/standard"
	const otherName = "feature/other"
	require.NoError(t, repository.runTimber(t, "create", at(testRepoName, standardName)).err)
	require.NoError(t, repository.runTimber(t, "create", at("aaa", otherName)).err)
	customPath := filepath.Join(resolvedTempDir(t), "checkout with spaces")
	runGitCommand(t, repository.barePath, "worktree", "add", "-b", "feature/manual", customPath, "origin/main")

	standardLocation := filepath.Join("~", "worktrees", testRepoName, standardName, testRepoName)
	otherLocation := filepath.Join("~", "worktrees", "aaa", otherName, "aaa")

	byRepo := repository.runTimber(t, "ls", "--locations-only", "--sort", "repo")
	require.NoError(t, byRepo.err, byRepo.stderr)
	assert.Equal(t, strings.Join([]string{otherLocation, customPath, standardLocation}, "\n")+"\n", byRepo.stdout)

	byWorktree := repository.runTimber(t, "ls", "--locations-only", "--sort", "worktree")
	require.NoError(t, byWorktree.err, byWorktree.stderr)
	assert.Equal(t, strings.Join([]string{customPath, otherLocation, standardLocation}, "\n")+"\n", byWorktree.stdout)

	scoped := repository.runTimber(t, "ls", at("aaa", ""), "--locations-only")
	require.NoError(t, scoped.err, scoped.stderr)
	assert.Equal(t, otherLocation+"\n", scoped.stdout)

	// Recency sorting must still order by committer date even when status
	// enrichment is skipped and a checkout directory is missing.
	for _, commit := range []struct{ dir, committer string }{
		{customPath, "2020-01-01T00:00:00Z"},
		{repoTestWorktreePath(repository.runtime, "aaa", otherName), "2021-01-01T00:00:00Z"},
		{repository.worktreePath(standardName), "2022-01-01T00:00:00Z"},
	} {
		runtime := withTestEnvironment(repository.runtime,
			"GIT_AUTHOR_DATE="+commit.committer, "GIT_COMMITTER_DATE="+commit.committer,
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		command := gitCommand(t, commit.dir, "commit", "--allow-empty", "-m", "order")
		command.Env = runtime.Environment
		output, err := command.CombinedOutput()
		require.NoError(t, err, string(output))
	}
	require.NoError(t, os.RemoveAll(repository.worktreePath(standardName)))

	byRecency := repository.runTimber(t, "ls", "--locations-only")
	require.NoError(t, byRecency.err, byRecency.stderr)
	assert.Equal(t, strings.Join([]string{standardLocation, otherLocation, customPath}, "\n")+"\n", byRecency.stdout)

	for _, conflict := range []struct {
		flag string
		name string
	}{
		{"--json", "json"},
		{"--pr", "pr"},
		{"--todo", "todo"},
		{"-t", "todo"},
		{"--review", "review"},
		{"-r", "review"},
		{"--location", "location"},
		{"-L", "location"},
	} {
		result := repository.runTimber(t, "ls", "--locations-only", conflict.flag)
		require.Error(t, result.err, "flag=%s", conflict.flag)
		assert.Contains(t, result.err.Error(), "locations-only")
		assert.Contains(t, result.err.Error(), conflict.name)
	}
}

func TestListReportsTodoProgress(t *testing.T) {
	t.Parallel()
	const branchName = "feature/todo-progress"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/no-todo")).err)
	writeTodoFile(t, todoPathForWorktree(t, testRepository.worktreePath(branchName)), "- [x] done\n- [ ] pending\n")

	for _, testCase := range []struct {
		flags   []string
		enabled bool
	}{
		{},
		{flags: []string{"-t"}, enabled: true},
		{flags: []string{"--todo"}, enabled: true},
		{flags: []string{"--todo=false"}},
	} {
		args := append([]string{"ls", at(testRepoName, "")}, testCase.flags...)
		result := testRepository.runTimber(t, args...)
		require.NoError(t, result.err, result.stderr)
		if testCase.enabled {
			assert.Contains(t, result.stdout, "Todo")
			assert.Contains(t, result.stdout, "1/2")
		} else {
			assert.NotContains(t, result.stdout, "Todo")
			assert.NotContains(t, result.stdout, "1/2")
		}
		encoded := testRepository.runTimber(t, append(args, "--json")...)
		require.NoError(t, encoded.err, encoded.stderr)
		assert.Contains(t, encoded.stdout, `"todoDone":1,"todoTotal":2`)
		assert.Contains(t, encoded.stdout, `"todoDone":0,"todoTotal":0`)
	}
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

	result := testRepository.runTimber(t, "list", "--json", "--todo", at(testRepoName, ""))
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
