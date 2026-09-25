package timber

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

type commandResult struct {
	stdout string
	stderr string
	err    error
}

type stubPrompter struct {
	selected []managedWorktree
	err      error
}

type stubCreateWizardPrompter struct {
	selection createWizardSelection
	err       error
	repos     []registeredRepo
	worktrees []managedWorktree
	showTitle bool
}

func (x stubPrompter) Prompt(input io.Reader, output io.Writer, worktrees []managedWorktree) ([]managedWorktree, error) {
	return x.selected, x.err
}

func (x *stubCreateWizardPrompter) Prompt(
	input io.Reader,
	output io.Writer,
	repos []registeredRepo,
	worktrees []managedWorktree,
	showTitle bool,
) (createWizardSelection, error) {
	x.repos = repos
	x.worktrees = worktrees
	x.showTitle = showTitle
	return x.selection, x.err
}

func installFakeHerdrSpace(t *testing.T, logPath string) string {
	t.Helper()

	binDir := resolvedTempDir(t)
	scriptPath := filepath.Join(binDir, "herdr")
	script := fmt.Sprintf(`#!/bin/sh
first=1
for arg in "$@"; do
  if [ "$first" -eq 1 ]; then
    first=0
  else
    printf '\037' >> %q
  fi
  printf '%%s' "$arg" >> %q
done
printf '\n' >> %q

operation="$1 $2"
tab_label=""
workspace_id=""
list_cwd=""
open_path=""
previous=""
for arg in "$@"; do
  if [ "$previous" = "--label" ]; then
    tab_label="$arg"
  elif [ "$previous" = "--workspace" ]; then
    workspace_id="$arg"
  elif [ "$previous" = "--cwd" ]; then
    list_cwd="$arg"
  elif [ "$previous" = "--path" ]; then
    open_path="$arg"
  fi
  previous="$arg"
done
if [ "$operation" = "${FAKE_HERDR_FAIL:-}" ] ||
   { [ "$operation" = "tab create" ] && [ "$tab_label" = "${FAKE_HERDR_FAIL_TAB_LABEL:-}" ]; }; then
  echo "fake herdr failure" >&2
  exit 1
fi
if [ "$operation" = "${FAKE_HERDR_MALFORM:-}" ]; then
  printf '{'
  exit 0
fi

case "$operation" in
  "workspace create")
    printf '{"result":{"workspace":{"workspace_id":"w1"},"tab":{"tab_id":"w1:t1"},"root_pane":{"pane_id":"w1:p1"}}}'
    ;;
  "workspace get")
    printf '{"result":{"workspace":{"workspace_id":"%%s","label":"%%s"}}}' "$3" "${FAKE_HERDR_PARENT_LABEL:-repo}"
    ;;
  "worktree list")
    if [ -z "${FAKE_HERDR_PARENT_ID:-}" ]; then
      parent_ref="null"
    else
      parent_ref="\"$FAKE_HERDR_PARENT_ID\""
    fi
    printf '{"result":{"source":{"repo_key":"test","repo_name":"repo","repo_root":"%%s","source_checkout_path":"%%s","source_workspace_id":%%s},"worktrees":[]}}' "$list_cwd" "$list_cwd" "$parent_ref"
    ;;
  "worktree open")
    if [ "${FAKE_HERDR_ALREADY_OPEN:-}" = "1" ]; then
      already="true"
    else
      already="false"
    fi
    printf '{"result":{"workspace":{"workspace_id":"w2"},"tab":{"tab_id":"w2:t1"},"root_pane":{"pane_id":"w2:p1"},"worktree":{"path":"%%s"},"already_open":%%s}}' "$open_path" "$already"
    ;;
  "pane current")
    printf '{"result":{"pane":{"workspace_id":"w9","tab_id":"w9:t1","pane_id":"w9:p1"}}}'
    ;;
  "tab create")
    if [ "$tab_label" = "Shell" ]; then
      printf '{"result":{"tab":{"tab_id":"%%s:t3"},"root_pane":{"pane_id":"%%s:p3"}}}' "$workspace_id" "$workspace_id"
    else
      printf '{"result":{"tab":{"tab_id":"%%s:t2"},"root_pane":{"pane_id":"%%s:p2"}}}' "$workspace_id" "$workspace_id"
    fi
    ;;
  *)
    printf '{"result":{}}'
    ;;
esac
`, logPath, logPath, logPath)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	return scriptPath
}

func installFakeGh(t *testing.T) string {
	t.Helper()

	binDir := resolvedTempDir(t)
	scriptPath := filepath.Join(binDir, "gh")
	script := `#!/bin/sh
operation="$1 $2"
if [ "$operation" = "auth status" ]; then
  if [ "${FAKE_GH_AUTH_OK:-1}" = "1" ]; then
    exit 0
  fi
  echo "not logged in" >&2
  exit 1
fi
if [ "$operation" = "pr list" ]; then
  printf '%s' "${FAKE_GH_PR_JSON:-[]}"
  exit 0
fi
echo "unexpected gh call: $*" >&2
exit 1
`
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	return scriptPath
}

func fakeHerdrLogLine(args ...string) string {
	return strings.Join(args, "\x1f")
}

func readFakeHerdrLog(t *testing.T, logPath string) []string {
	t.Helper()
	contents, err := os.ReadFile(logPath)
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(contents)), "\n")
}

func skipIfNoPty(t *testing.T) {
	t.Helper()
	command := testCommand(t, "python3", "-c", "import pty; pty.openpty()")
	if err := command.Run(); err != nil {
		t.Skip("pty devices are not available")
	}
}

func testRuntime(t *testing.T) Runtime {
	t.Helper()
	return testRuntimeForHome(resolvedTempDir(t), "")
}

func testRuntimeForHome(home string, currentDirectory string) Runtime {
	if currentDirectory == "" {
		currentDirectory = home
	}
	dataHome := filepath.Join(home, ".local", "share")
	return Runtime{
		CurrentDirectory:   currentDirectory,
		HomeDirectory:      home,
		DataHome:           dataHome,
		ConfigHome:         filepath.Join(home, ".config"),
		WorktreeRoot:       filepath.Join(home, worktreesDirName),
		TemporaryDirectory: os.TempDir(),
		Environment:        testEnvironment(home, dataHome),
	}
}

func testEnvironment(home string, dataHome string) []string {
	return replaceTestEnvironment(gitTestEnv(),
		"HOME="+home,
		"XDG_DATA_HOME="+dataHome,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		worktreeRootEnvVarName+"="+filepath.Join(home, worktreesDirName),
	)
}

func replaceTestEnvironment(environment []string, replacements ...string) []string {
	keys := make(map[string]struct{}, len(replacements))
	for _, replacement := range replacements {
		key, _, _ := strings.Cut(replacement, "=")
		keys[key] = struct{}{}
	}

	updated := make([]string, 0, len(environment)+len(replacements))
	for _, value := range environment {
		key, _, _ := strings.Cut(value, "=")
		if _, replace := keys[key]; !replace {
			updated = append(updated, value)
		}
	}
	return append(updated, replacements...)
}

func withTestEnvironment(runtime Runtime, replacements ...string) Runtime {
	runtime.Environment = replaceTestEnvironment(runtime.Environment, replacements...)
	return runtime
}

func newTestRootCommand(t *testing.T) *cobra.Command {
	t.Helper()
	return NewRootCommand(testRuntime(t))
}

func newTestRootCommandWithRuntime(t *testing.T, runtime Runtime) *cobra.Command {
	t.Helper()
	return NewRootCommand(runtime)
}

func runCompleteWithRuntime(t *testing.T, runtime Runtime, args ...string) string {
	t.Helper()
	command := newTestRootCommandWithRuntime(t, runtime)
	command.SetArgs(append([]string{"__complete"}, args...))
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(io.Discard)
	require.NoError(t, command.Execute())
	return stdout.String()
}

func mustResolveRemoteURL(t *testing.T, input string) string {
	t.Helper()
	resolved, err := resolveRemoteURL(input)
	require.NoError(t, err)
	return resolved
}

func at(repo string, name string) string {
	if name == "" {
		return "@" + repo
	}
	return name + "@" + repo
}

const testRepoName = "repo"

type testRepository struct {
	runtime      Runtime
	home         string
	barePath     string
	remotePath   string
	worktreeRoot string
}

type testRepositoryFixture struct {
	root       string
	remotePath string
	barePath   string
}

type testRepositoryFixtureResult struct {
	fixture testRepositoryFixture
	err     error
}

var testRepositoryFixtureRoot string

var getTestRepositoryFixture = sync.OnceValue(func() testRepositoryFixtureResult {
	fixture, err := createTestRepositoryFixture()
	if err == nil {
		testRepositoryFixtureRoot = fixture.root
	}
	return testRepositoryFixtureResult{fixture: fixture, err: err}
})

func TestMain(m *testing.M) {
	status := m.Run()
	if testRepositoryFixtureRoot != "" {
		_ = os.RemoveAll(testRepositoryFixtureRoot)
	}
	os.Exit(status)
}

func createTestRepositoryFixture() (testRepositoryFixture, error) {
	root, err := os.MkdirTemp("", "timber-test-fixture-")
	if err != nil {
		return testRepositoryFixture{}, err
	}
	removeRoot := true
	defer func() {
		if removeRoot {
			_ = os.RemoveAll(root)
		}
	}()

	runGit := func(cwd string, args ...string) error {
		_, err := runGitCommandResult(cwd, args...)
		if err != nil {
			return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return nil
	}

	remotePath := filepath.Join(root, "remote.git")
	if err := runGit(root, "init", "--bare", remotePath); err != nil {
		return testRepositoryFixture{}, err
	}

	seedPath := filepath.Join(root, "seed")
	if err := runGit(root, "clone", remotePath, seedPath); err != nil {
		return testRepositoryFixture{}, err
	}
	if err := os.WriteFile(filepath.Join(seedPath, "README.md"), []byte("initial\n"), 0o644); err != nil {
		return testRepositoryFixture{}, err
	}
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-m", "initial"},
		{"branch", "-M", "main"},
		{"push", "-u", remoteName, "main"},
	} {
		if err := runGit(seedPath, args...); err != nil {
			return testRepositoryFixture{}, err
		}
	}
	// git init --bare leaves HEAD at init.defaultBranch (often master). Point it at
	// the branch we actually pushed so clones and remote set-head --auto work on CI.
	if err := runGit(remotePath, "symbolic-ref", "HEAD", "refs/heads/main"); err != nil {
		return testRepositoryFixture{}, err
	}

	barePath := filepath.Join(root, "bare.git")
	if err := runGit(root, "clone", "--bare", remotePath, barePath); err != nil {
		return testRepositoryFixture{}, err
	}
	for _, args := range [][]string{
		{"remote", "remove", remoteName},
		{"remote", "add", remoteName, remotePath},
		{"fetch", remoteName},
		{"remote", "set-head", remoteName, "main"},
	} {
		if err := runGit(barePath, args...); err != nil {
			return testRepositoryFixture{}, err
		}
	}

	removeRoot = false
	return testRepositoryFixture{root: root, remotePath: remotePath, barePath: barePath}, nil
}

func newTestRepository(t *testing.T) testRepository {
	t.Helper()

	fixture := getTestRepositoryFixture()
	require.NoError(t, fixture.err)

	home := resolvedTempDir(t)
	worktreeRootPath := filepath.Join(home, "worktrees")
	runtime := testRuntimeForHome(home, home)

	remoteParent := resolvedTempDir(t)
	remotePath := filepath.Join(remoteParent, "remote.git")
	require.NoError(t, os.CopyFS(remotePath, os.DirFS(fixture.fixture.remotePath)))

	reposDir := filepath.Join(home, ".local", "share", "timber", "repos")
	require.NoError(t, os.MkdirAll(reposDir, 0o755))
	barePath := filepath.Join(reposDir, testRepoName+".git")
	require.NoError(t, os.CopyFS(barePath, os.DirFS(fixture.fixture.barePath)))
	require.NoError(t, replaceGitRemotePath(barePath, fixture.fixture.remotePath, remotePath))

	return testRepository{
		runtime:      runtime,
		home:         home,
		barePath:     barePath,
		remotePath:   remotePath,
		worktreeRoot: worktreeRootPath,
	}
}

// registerAdditionalRepo copies another bare repo into the same registry home as base.
func registerAdditionalRepo(t *testing.T, base testRepository, name string) string {
	t.Helper()

	fixture := getTestRepositoryFixture()
	require.NoError(t, fixture.err)

	reposDir := filepath.Join(base.home, ".local", "share", "timber", "repos")
	barePath := filepath.Join(reposDir, filepath.FromSlash(name)+".git")
	require.NoError(t, os.MkdirAll(filepath.Dir(barePath), 0o755))
	require.NoError(t, os.CopyFS(barePath, os.DirFS(fixture.fixture.barePath)))
	require.NoError(t, replaceGitRemotePath(barePath, fixture.fixture.remotePath, base.remotePath))
	return barePath
}

func replaceGitRemotePath(barePath string, oldPath string, newPath string) error {
	configPath := filepath.Join(barePath, "config")
	contents, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	updated := bytes.Replace(contents, []byte(oldPath), []byte(newPath), 1)
	if bytes.Equal(contents, updated) {
		return fmt.Errorf("git remote path %q not found in %s", oldPath, configPath)
	}
	return os.WriteFile(configPath, updated, 0o644)
}

func seedBareRemote(t *testing.T, remotePath string) {
	t.Helper()
	tempClone := filepath.Join(resolvedTempDir(t), "seed")
	runGitCommand(t, filepath.Dir(tempClone), "clone", remotePath, tempClone)
	configureGitUser(t, tempClone)
	require.NoError(t, os.WriteFile(filepath.Join(tempClone, "README.md"), []byte("initial\n"), 0o644))
	runGitCommand(t, tempClone, "add", "README.md")
	runGitCommand(t, tempClone, "commit", "-m", "initial")
	runGitCommand(t, tempClone, "branch", "-M", "main")
	runGitCommand(t, tempClone, "push", "-u", remoteName, "main")
	// git init --bare leaves HEAD at init.defaultBranch (often master). Point it at
	// the branch we actually pushed so clones and remote set-head --auto work on CI.
	runGitCommand(t, remotePath, "symbolic-ref", "HEAD", "refs/heads/main")
}

func configureGitUser(t *testing.T, path string) {
	t.Helper()
	runGitCommand(t, path, "config", "user.name", "Test User")
	runGitCommand(t, path, "config", "user.email", "test@example.com")
}

func (x testRepository) worktreePath(branchName string) string {
	return filepath.Join(x.worktreeRoot, testRepoName, branchName, testRepoName)
}

func (x testRepository) runTimber(t *testing.T, args ...string) commandResult {
	t.Helper()
	return x.runTimberFrom(t, x.home, args...)
}

func (x testRepository) runTimberFrom(t *testing.T, directory string, args ...string) commandResult {
	t.Helper()

	runtime := x.runtime
	runtime.CurrentDirectory = directory
	return runTimberCommandWithRuntime(t, runtime, args...)
}

func runTimberFromWithRuntime(t *testing.T, runtime Runtime, directory string, args ...string) commandResult {
	t.Helper()

	runtime.CurrentDirectory = directory
	return runTimberCommandWithRuntime(t, runtime, args...)
}

func runTimberCommand(t *testing.T, args ...string) commandResult {
	t.Helper()
	return runTimberCommandWithRuntime(t, testRuntime(t), args...)
}

func runTimberCommandWithRuntime(t *testing.T, runtime Runtime, args ...string) commandResult {
	t.Helper()

	command := newTestRootCommandWithRuntime(t, runtime)
	command.SetArgs(args)
	command.SetIn(bytes.NewBuffer(nil))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)

	err := command.Execute()
	return commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func runTUICreate(
	t *testing.T,
	options *tuiCreateCommandOptions,
	prompter createWizardPrompter,
) commandResult {
	t.Helper()

	options.prompter = prompter
	if options.runtime.CurrentDirectory == "" {
		options.runtime = testRuntime(t)
	}
	command := newTestRootCommandWithRuntime(t, options.runtime)
	command.SetContext(t.Context())
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)

	err := options.Execute(command, nil)
	return commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func (x testRepository) commitFileInWorktree(t *testing.T, branchName string, fileName string, contents string) {
	t.Helper()
	path := x.worktreePath(branchName)
	x.writeFileInWorktree(t, branchName, fileName, contents)
	runGitCommand(t, path, "add", fileName)
	runGitCommand(t, path, "commit", "-m", "change")
}

func (x testRepository) writeFileInWorktree(t *testing.T, branchName string, fileName string, contents string) {
	t.Helper()
	path := filepath.Join(x.worktreePath(branchName), fileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
}

func (x testRepository) mergeWorktreeBranch(t *testing.T, branchName string) {
	t.Helper()
	// Create a temporary worktree on main to merge into, then push.
	mergePath := filepath.Join(resolvedTempDir(t), "merge-main")
	runGitCommand(t, x.barePath, "worktree", "add", mergePath, "main")
	runGitCommand(t, mergePath, "merge", "--ff-only", branchName)
	runGitCommand(t, mergePath, "push", remoteName, "main")
	runGitCommand(t, x.barePath, "fetch", remoteName)
	runGitCommand(t, x.barePath, "worktree", "remove", mergePath)
}

func (x testRepository) assertBranchMissing(t *testing.T, branchName string) {
	t.Helper()
	command := testCommand(t, "git", "--git-dir", x.barePath, "show-ref", "--verify", "--quiet", "refs/heads/"+branchName)
	err := command.Run()
	if exitError, ok := errors.AsType[*exec.ExitError](err); ok && exitError.ExitCode() == 1 {
		return
	}
	if err == nil {
		require.Fail(t, fmt.Sprintf("expected branch %s to be missing", branchName))
	}
	require.Fail(t, fmt.Sprintf("unexpected error checking branch %s: %v", branchName, err))
}

func (x testRepository) assertPathMissing(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist, "expected path %s to be missing", path)
}

func (x testRepository) assertPathPresent(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	require.NoError(t, err, "expected path %s to be present", path)
}

func runGitCommand(t *testing.T, cwd string, args ...string) string {
	t.Helper()

	output, err := runGitCommandResult(cwd, args...)
	if err != nil {
		require.Fail(t, fmt.Sprintf("git %s failed: %v\n%s", strings.Join(args, " "), err, output))
	}

	return output
}

func runGitCommandResult(cwd string, args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = cwd
	command.Env = append(gitTestEnv(),
		"HOME="+cwd,
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)

	output, err := command.CombinedOutput()
	return string(output), err
}

// Start from an allowlist, not os.Environ: rebase Git overrides, user
// configuration, shell startup hooks, and Timber/Herdr state must not leak
// into fixtures. Callers supply a temporary HOME and any deliberate overrides.
func gitTestEnv() []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"TMPDIR=" + os.TempDir(),
		"LANG=C",
		"LC_ALL=C",
		"TERM=xterm-256color",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_ATTR_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT=3",
		"GIT_CONFIG_KEY_0=core.hooksPath",
		"GIT_CONFIG_VALUE_0=" + os.DevNull,
		"GIT_CONFIG_KEY_1=commit.gpgSign",
		"GIT_CONFIG_VALUE_1=false",
		"GIT_CONFIG_KEY_2=init.templateDir",
		"GIT_CONFIG_VALUE_2=",
	}
}

func runGitCommandAllowError(t *testing.T, cwd string, args ...string) {
	t.Helper()
	_, _ = runGitCommandResult(cwd, args...)
}

// resolvedTempDir returns a per-test temporary directory with symlinks resolved.
// On macOS resolvedTempDir(t) lives under /var, a symlink to /private/var, which
// breaks path comparisons against canonicalized paths reported by git and
// the shell.
func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}
