package timber

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parentDashboardLogLine() string {
	return fakeHerdrLogLine("pane", "run", "w1:p1", "while :; do clear; timber list '@"+testRepoName+"' --pr; sleep 60; done")
}

func parentDashboardLogLineWithoutPR() string {
	return fakeHerdrLogLine("pane", "run", "w1:p1", "while :; do clear; timber list '@"+testRepoName+"'; sleep 60; done")
}

func TestSetupSpaceOpensNamedWorktreeInNewHerdrWorkspace(t *testing.T) {
	t.Parallel()
	const branchName = "feature/space"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	logPath := filepath.Join(resolvedTempDir(t), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime.GhExecutable = installFakeGh(t)

	result := testRepository.runTimber(t, "herdr", "space", "--new", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "opened herdr space for "+branchName)

	worktreePath := canonicalPath(testRepository.worktreePath(branchName))
	barePath := canonicalPath(testRepository.barePath)
	assert.Equal(t, []string{
		fakeHerdrLogLine("worktree", "list", "--cwd", barePath),
		fakeHerdrLogLine("workspace", "create", "--cwd", barePath, "--label", testRepoName, "--no-focus"),
		fakeHerdrLogLine("tab", "rename", "w1:t1", "Status"),
		parentDashboardLogLine(),
		fakeHerdrLogLine("worktree", "open", "--workspace", "w1", "--path", worktreePath, "--label", branchName, "--no-focus"),
		fakeHerdrLogLine("tab", "rename", "w2:t1", "Agent"),
		fakeHerdrLogLine("pane", "rename", "w2:p1", branchName),
		fakeHerdrLogLine("tab", "create", "--workspace", "w2", "--cwd", worktreePath, "--label", "Shell", "--no-focus"),
		fakeHerdrLogLine("pane", "run", "w2:p1", "pi"),
		fakeHerdrLogLine("workspace", "focus", "w2"),
		fakeHerdrLogLine("tab", "focus", "w2:t1"),
	}, readFakeHerdrLog(t, logPath))
}

func TestSetupSpaceDashboardOmitsPullRequestsWithoutGhAuth(t *testing.T) {
	t.Parallel()
	const branchName = "feature/no-gh-auth"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	logPath := filepath.Join(resolvedTempDir(t), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime.GhExecutable = installFakeGh(t)
	testRepository.runtime = withTestEnvironment(testRepository.runtime, "FAKE_GH_AUTH_OK=0")

	result := testRepository.runTimber(t, "herdr", "space", "--new", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "opened herdr space for "+branchName)

	worktreePath := canonicalPath(testRepository.worktreePath(branchName))
	barePath := canonicalPath(testRepository.barePath)
	assert.Equal(t, []string{
		fakeHerdrLogLine("worktree", "list", "--cwd", barePath),
		fakeHerdrLogLine("workspace", "create", "--cwd", barePath, "--label", testRepoName, "--no-focus"),
		fakeHerdrLogLine("tab", "rename", "w1:t1", "Status"),
		parentDashboardLogLineWithoutPR(),
		fakeHerdrLogLine("worktree", "open", "--workspace", "w1", "--path", worktreePath, "--label", branchName, "--no-focus"),
		fakeHerdrLogLine("tab", "rename", "w2:t1", "Agent"),
		fakeHerdrLogLine("pane", "rename", "w2:p1", branchName),
		fakeHerdrLogLine("tab", "create", "--workspace", "w2", "--cwd", worktreePath, "--label", "Shell", "--no-focus"),
		fakeHerdrLogLine("pane", "run", "w2:p1", "pi"),
		fakeHerdrLogLine("workspace", "focus", "w2"),
		fakeHerdrLogLine("tab", "focus", "w2:t1"),
	}, readFakeHerdrLog(t, logPath))
}

func TestSetupSpaceReusesExistingParentWorkspace(t *testing.T) {
	t.Parallel()
	const branchName = "feature/shared-parent"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	logPath := filepath.Join(resolvedTempDir(t), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime.GhExecutable = installFakeGh(t)
	testRepository.runtime = withTestEnvironment(testRepository.runtime, "FAKE_HERDR_PARENT_ID=w7")

	result := testRepository.runTimber(t, "herdr", "space", "--new", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "opened herdr space for "+branchName)

	worktreePath := canonicalPath(testRepository.worktreePath(branchName))
	barePath := canonicalPath(testRepository.barePath)
	assert.Equal(t, []string{
		fakeHerdrLogLine("worktree", "list", "--cwd", barePath),
		fakeHerdrLogLine("workspace", "get", "w7"),
		fakeHerdrLogLine("worktree", "open", "--workspace", "w7", "--path", worktreePath, "--label", branchName, "--no-focus"),
		fakeHerdrLogLine("tab", "rename", "w2:t1", "Agent"),
		fakeHerdrLogLine("pane", "rename", "w2:p1", branchName),
		fakeHerdrLogLine("tab", "create", "--workspace", "w2", "--cwd", worktreePath, "--label", "Shell", "--no-focus"),
		fakeHerdrLogLine("pane", "run", "w2:p1", "pi"),
		fakeHerdrLogLine("workspace", "focus", "w2"),
		fakeHerdrLogLine("tab", "focus", "w2:t1"),
	}, readFakeHerdrLog(t, logPath))
}

func TestSetupSpaceRenamesLegacyParentLabel(t *testing.T) {
	t.Parallel()
	const branchName = "feature/legacy-parent"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	logPath := filepath.Join(resolvedTempDir(t), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime.GhExecutable = installFakeGh(t)
	testRepository.runtime = withTestEnvironment(
		testRepository.runtime,
		"FAKE_HERDR_PARENT_ID=w7",
		"FAKE_HERDR_PARENT_LABEL="+testRepoName+".git",
	)

	result := testRepository.runTimber(t, "herdr", "space", "--new", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)

	worktreePath := canonicalPath(testRepository.worktreePath(branchName))
	barePath := canonicalPath(testRepository.barePath)
	assert.Equal(t, []string{
		fakeHerdrLogLine("worktree", "list", "--cwd", barePath),
		fakeHerdrLogLine("workspace", "get", "w7"),
		fakeHerdrLogLine("workspace", "rename", "w7", testRepoName),
		fakeHerdrLogLine("worktree", "open", "--workspace", "w7", "--path", worktreePath, "--label", branchName, "--no-focus"),
		fakeHerdrLogLine("tab", "rename", "w2:t1", "Agent"),
		fakeHerdrLogLine("pane", "rename", "w2:p1", branchName),
		fakeHerdrLogLine("tab", "create", "--workspace", "w2", "--cwd", worktreePath, "--label", "Shell", "--no-focus"),
		fakeHerdrLogLine("pane", "run", "w2:p1", "pi"),
		fakeHerdrLogLine("workspace", "focus", "w2"),
		fakeHerdrLogLine("tab", "focus", "w2:t1"),
	}, readFakeHerdrLog(t, logPath))
}

func TestSetupSpaceFocusesAlreadyOpenWorktreeWithoutReconfiguring(t *testing.T) {
	t.Parallel()
	const branchName = "feature/already-open"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	logPath := filepath.Join(resolvedTempDir(t), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime.GhExecutable = installFakeGh(t)
	testRepository.runtime = withTestEnvironment(testRepository.runtime, "FAKE_HERDR_ALREADY_OPEN=1")

	result := testRepository.runTimber(t, "herdr", "space", "--new", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "opened herdr space for "+branchName)

	worktreePath := canonicalPath(testRepository.worktreePath(branchName))
	barePath := canonicalPath(testRepository.barePath)
	assert.Equal(t, []string{
		fakeHerdrLogLine("worktree", "list", "--cwd", barePath),
		fakeHerdrLogLine("workspace", "create", "--cwd", barePath, "--label", testRepoName, "--no-focus"),
		fakeHerdrLogLine("tab", "rename", "w1:t1", "Status"),
		parentDashboardLogLine(),
		fakeHerdrLogLine("worktree", "open", "--workspace", "w1", "--path", worktreePath, "--label", branchName, "--no-focus"),
		fakeHerdrLogLine("workspace", "focus", "w2"),
	}, readFakeHerdrLog(t, logPath))
}

func TestSetupSpaceDefinesNamedWorktreeTabsInCurrentHerdrSpace(t *testing.T) {
	t.Parallel()
	const branchName = "feature/current-herdr-space"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	logPath := filepath.Join(resolvedTempDir(t), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime.GhExecutable = installFakeGh(t)

	result := testRepository.runTimber(t, "herdr", "space", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "defined herdr tabs in current space for "+branchName)

	worktreePath := canonicalPath(testRepository.worktreePath(branchName))
	assert.Equal(t, []string{
		fakeHerdrLogLine("pane", "current", "--current"),
		fakeHerdrLogLine("tab", "rename", "w9:t1", "Agent"),
		fakeHerdrLogLine("pane", "rename", "w9:p1", branchName),
		fakeHerdrLogLine("tab", "create", "--workspace", "w9", "--cwd", worktreePath, "--label", "Shell", "--no-focus"),
		fakeHerdrLogLine("pane", "run", "w9:p1", "pi"),
		fakeHerdrLogLine("workspace", "focus", "w9"),
		fakeHerdrLogLine("tab", "focus", "w9:t1"),
	}, readFakeHerdrLog(t, logPath))
}

func TestSetupSpaceDoesNotCloseCurrentHerdrSpaceWhenTabCreationFails(t *testing.T) {
	t.Parallel()
	const branchName = "feature/current-herdr-space-failure"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	logPath := filepath.Join(resolvedTempDir(t), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime.GhExecutable = installFakeGh(t)
	testRepository.runtime = withTestEnvironment(testRepository.runtime, "FAKE_HERDR_FAIL=tab create")

	result := testRepository.runTimber(t, "herdr", "space", at(testRepoName, branchName))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "herdr tab create")
	assert.NotContains(t, readFakeHerdrLog(t, logPath), fakeHerdrLogLine("workspace", "close", "w9"))
}

func TestSetupSpaceUsesCurrentWorktreeFromSubdirectory(t *testing.T) {
	t.Parallel()
	const branchName = "feature/current-space"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	subdirectory := filepath.Join(testRepository.worktreePath(branchName), "nested")
	require.NoError(t, os.MkdirAll(subdirectory, 0o755))
	logPath := filepath.Join(resolvedTempDir(t), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime.GhExecutable = installFakeGh(t)

	result := testRepository.runTimberFrom(t, subdirectory, "herdr", "space")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, readFakeHerdrLog(t, logPath), fakeHerdrLogLine(
		"tab", "create", "--workspace", "w9", "--cwd", canonicalPath(testRepository.worktreePath(branchName)),
		"--label", "Shell", "--no-focus",
	))
}

func TestSetupSpaceFailsForUnknownWorktree(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	result := testRepository.runTimber(t, "herdr", "space", at(testRepoName, "feature/missing"))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), `unknown worktree "feature/missing"`)
}

func TestSetupSpaceRequiresNameOutsideManagedWorktree(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	result := testRepository.runTimber(t, "herdr", "space", at(testRepoName, ""))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "worktree name is required")
}

func TestSetupSpaceClosesNewWorkspaceWhenTabCreationFails(t *testing.T) {
	t.Parallel()
	const branchName = "feature/space-failure"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	logPath := filepath.Join(resolvedTempDir(t), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime.GhExecutable = installFakeGh(t)
	testRepository.runtime = withTestEnvironment(testRepository.runtime, "FAKE_HERDR_FAIL=tab create")

	result := testRepository.runTimber(t, "herdr", "space", "--new", at(testRepoName, branchName))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "herdr tab create")
	assert.Equal(t, fakeHerdrLogLine("workspace", "close", "w2"), readFakeHerdrLog(t, logPath)[8])
}

func TestSetupSpaceClosesNewWorkspaceWhenShellTabCreationFails(t *testing.T) {
	t.Parallel()
	const branchName = "feature/space-shell-failure"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	logPath := filepath.Join(resolvedTempDir(t), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime.GhExecutable = installFakeGh(t)
	testRepository.runtime = withTestEnvironment(testRepository.runtime, "FAKE_HERDR_FAIL_TAB_LABEL=Shell")

	result := testRepository.runTimber(t, "herdr", "space", "-n", at(testRepoName, branchName))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "herdr tab create")
	assert.Equal(t, fakeHerdrLogLine("workspace", "close", "w2"), readFakeHerdrLog(t, logPath)[8])
}

func TestSetupSpaceClosesNewWorkspaceWhenTabResponseIsInvalid(t *testing.T) {
	t.Parallel()
	const branchName = "feature/space-invalid-response"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	logPath := filepath.Join(resolvedTempDir(t), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime.GhExecutable = installFakeGh(t)
	testRepository.runtime = withTestEnvironment(testRepository.runtime, "FAKE_HERDR_MALFORM=tab create")

	result := testRepository.runTimber(t, "herdr", "space", "--new", at(testRepoName, branchName))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "decode herdr tab create response")
	assert.Equal(t, fakeHerdrLogLine("workspace", "close", "w2"), readFakeHerdrLog(t, logPath)[8])
}

func TestSetupSpaceFailsWhenParentLookupFails(t *testing.T) {
	t.Parallel()
	const branchName = "feature/space-parent-failure"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	logPath := filepath.Join(resolvedTempDir(t), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime.GhExecutable = installFakeGh(t)
	testRepository.runtime = withTestEnvironment(testRepository.runtime, "FAKE_HERDR_FAIL=worktree list")

	result := testRepository.runTimber(t, "herdr", "space", "--new", at(testRepoName, branchName))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "herdr worktree list")
	assert.Equal(t, []string{
		fakeHerdrLogLine("worktree", "list", "--cwd", canonicalPath(testRepository.barePath)),
	}, readFakeHerdrLog(t, logPath))
}

func TestSetupSpaceCompletionOffersManagedWorktreeNames(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/a")).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/b")).err)

	stdout := runCompleteWithRuntime(t, testRepository.runtime, "herdr", "space", "")
	assert.Contains(t, stdout, "feature/a")
	assert.Contains(t, stdout, "feature/b")
}
