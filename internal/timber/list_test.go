package timber

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupListTableRowsAddsRuleAfterEverySecondWorktree(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		worktrees          []managedWorktree
		groupedNames       []string
		horizontalRuleRows int
	}{
		{
			name: "odd number of worktrees",
			worktrees: []managedWorktree{
				{Name: "one", Clean: true},
				{Name: "two", Clean: true},
				{Name: "three", Clean: true},
				{Name: "four", Clean: true},
				{Name: "five", Clean: true},
			},
			groupedNames:       []string{"one\ntwo", "three\nfour", "five"},
			horizontalRuleRows: 3,
		},
		{
			name: "even number of worktrees",
			worktrees: []managedWorktree{
				{Name: "one", Clean: true},
				{Name: "two", Clean: true},
				{Name: "three", Clean: true},
				{Name: "four", Clean: true},
			},
			groupedNames:       []string{"one\ntwo", "three\nfour"},
			horizontalRuleRows: 2,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			rows := groupListTableRows(testCase.worktrees, newListStatusFormatter(testCase.worktrees), false)
			require.Len(t, rows, len(testCase.groupedNames))
			for index, names := range testCase.groupedNames {
				assert.Equal(t, names, rows[index][0])
			}

			tableView := newOutputTable("Name", "Repo", "Status", "Commit", "Dirty", "Merged").BorderRow(true)
			tableView.Rows(rows...)
			tableOutput := dottedListRowRules(tableView.String())
			assert.Equal(t, testCase.horizontalRuleRows, strings.Count(tableOutput, "├"))
			assert.Equal(t, 1, strings.Count(tableOutput, "├─"))
			assert.Equal(t, testCase.horizontalRuleRows-1, strings.Count(tableOutput, "├┈"))
		})
	}
}

func TestGroupListTableRowsIncludesPullRequestColumnWhenEnabled(t *testing.T) {
	t.Parallel()
	worktrees := []managedWorktree{
		{Name: "one", Clean: true, PullRequest: "#42 ✓"},
		{Name: "two", Clean: true},
	}

	rows := groupListTableRows(worktrees, newListStatusFormatter(worktrees), true)
	require.Len(t, rows, 1)
	assert.Equal(t, "#42 ✓\n", rows[0][6])

	rows = groupListTableRows(worktrees, newListStatusFormatter(worktrees), false)
	require.Len(t, rows, 1)
	assert.Len(t, rows[0], 6)
}

func TestFormatDirtyStatusColorsTrueYellow(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "false", formatDirtyStatus(true))
	assert.Equal(t, warningStyle.Render("true"), formatDirtyStatus(false))
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
	assert.Contains(t, result.stdout, "Merged")
	assert.Contains(t, result.stdout, "feature/fresh")
	assert.Contains(t, result.stdout, "feature/ahead")
	assert.Contains(t, result.stdout, "true")
	assert.Contains(t, result.stdout, "false")
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
	assert.Contains(t, result.stdout, "true")
}

func TestListOutsideManagedWorktreeListsAllRepos(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	secondaryName := "other"
	secondaryBare := registerAdditionalRepo(t, primary, secondaryName)

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
