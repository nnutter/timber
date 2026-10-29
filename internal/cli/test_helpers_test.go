package cli

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/cli/root"
	"github.com/nnutter/timber/internal/timber"
)

// These fixtures use the production command tree. They do not depend on
// application internals or a second implementation of command assembly.
func newRootCommand(runtime timber.Runtime) *cobra.Command {
	return root.New(runtime.HomeDirectory, runtime.SkillContent, Commands(runtime)...)
}

type commandResult struct {
	stdout string
	stderr string
	err    error
}

func runTimberCommand(t *testing.T, args ...string) commandResult {
	t.Helper()
	return runTimberCommandWithRuntime(t, testRuntime(t), args...)
}

func runTimberCommandWithRuntime(t *testing.T, runtime timber.Runtime, args ...string) commandResult {
	t.Helper()
	command := newRootCommand(runtime)
	command.SetArgs(args)
	command.SetIn(bytes.NewReader(nil))
	var stdout, stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	err := command.Execute()
	return commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func runTimberFromWithRuntime(t *testing.T, runtime timber.Runtime, directory string, args ...string) commandResult {
	t.Helper()
	runtime.CurrentDirectory = directory
	return runTimberCommandWithRuntime(t, runtime, args...)
}

func runCompleteWithRuntime(t *testing.T, runtime timber.Runtime, args ...string) string {
	t.Helper()
	command := newRootCommand(runtime)
	command.SetArgs(append([]string{"__complete"}, args...))
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(io.Discard)
	require.NoError(t, command.Execute())
	return stdout.String()
}

func testRuntime(t *testing.T) timber.Runtime {
	t.Helper()
	home := resolvedTempDir(t)
	return testRuntimeForHome(home, home)
}

func testRuntimeForHome(home, directory string) timber.Runtime {
	if directory == "" {
		directory = home
	}
	dataHome := filepath.Join(home, ".local", "share")
	configHome := filepath.Join(home, ".config")
	worktreeRoot := filepath.Join(home, "worktrees")
	return timber.Runtime{
		CurrentDirectory:   directory,
		HomeDirectory:      home,
		DataHome:           dataHome,
		ConfigHome:         configHome,
		WorktreeRoot:       worktreeRoot,
		TemporaryDirectory: os.TempDir(),
		Environment: append(gitTestEnv(),
			"HOME="+home,
			"XDG_DATA_HOME="+dataHome,
			"XDG_CONFIG_HOME="+configHome,
			"TIMBER_WORKTREE_ROOT="+worktreeRoot,
			// Fixtures can live inside this checkout. Do not discover its
			// repository when the command runs outside a fixture worktree.
			"GIT_CEILING_DIRECTORIES="+filepath.Dir(home),
		),
	}
}

func withTestEnvironment(runtime timber.Runtime, replacements ...string) timber.Runtime {
	runtime.Environment = shellTestEnvironment(runtime.Environment, replacements...)
	return runtime
}

const testRepoName = "repo"

type testRepository struct {
	runtime      timber.Runtime
	home         string
	barePath     string
	remotePath   string
	worktreeRoot string
}

func newTestRepository(t *testing.T) testRepository {
	t.Helper()
	runtime := testRuntime(t)
	remoteParent := resolvedTempDir(t)
	remotePath := filepath.Join(remoteParent, "remote.git")
	runGitCommand(t, remoteParent, "init", "--bare", remotePath)
	seedPath := filepath.Join(resolvedTempDir(t), "seed")
	runGitCommand(t, remoteParent, "clone", remotePath, seedPath)
	require.NoError(t, os.WriteFile(filepath.Join(seedPath, "README.md"), []byte("initial\n"), 0o644))
	runGitCommand(t, seedPath, "add", "README.md")
	runGitCommand(t, seedPath, "commit", "-m", "initial")
	runGitCommand(t, seedPath, "branch", "-M", "main")
	runGitCommand(t, seedPath, "push", "-u", "origin", "main")
	runGitCommand(t, remotePath, "symbolic-ref", "HEAD", "refs/heads/main")

	repository := testRepository{
		runtime:      runtime,
		home:         runtime.HomeDirectory,
		barePath:     filepath.Join(runtime.WorktreeRoot, testRepoName, ".git"),
		remotePath:   remotePath,
		worktreeRoot: runtime.WorktreeRoot,
	}
	registerAdditionalRepo(t, repository, testRepoName)
	return repository
}

func registerAdditionalRepo(t *testing.T, repository testRepository, name string) {
	t.Helper()
	result := repository.runTimber(t, "repo", "add", "--name", name, repository.remotePath)
	require.NoError(t, result.err, result.stderr)
}

func (x testRepository) runTimber(t *testing.T, args ...string) commandResult {
	t.Helper()
	return runTimberCommandWithRuntime(t, x.runtime, args...)
}

func (x testRepository) runTimberFrom(t *testing.T, directory string, args ...string) commandResult {
	t.Helper()
	return runTimberFromWithRuntime(t, x.runtime, directory, args...)
}

func (x testRepository) commitFileInWorktree(t *testing.T, branch, name, contents string) {
	t.Helper()
	x.writeFileInWorktree(t, branch, name, contents)
	path := x.worktreePath(branch)
	runGitCommand(t, path, "add", name)
	runGitCommand(t, path, "commit", "-m", "change")
}

func (x testRepository) writeFileInWorktree(t *testing.T, branch, name, contents string) {
	t.Helper()
	path := filepath.Join(x.worktreePath(branch), name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
}

func (x testRepository) worktreePath(branch string) string {
	return filepath.Join(x.worktreeRoot, testRepoName, branch, testRepoName)
}

func (x testRepository) mergeWorktreeBranch(t *testing.T, branch string) {
	t.Helper()
	mergePath := filepath.Join(resolvedTempDir(t), "merge-main")
	runGitCommand(t, x.barePath, "worktree", "add", mergePath, "main")
	runGitCommand(t, mergePath, "merge", "--ff-only", branch)
	runGitCommand(t, mergePath, "push", "origin", "main")
	runGitCommand(t, x.barePath, "fetch", "origin")
	runGitCommand(t, x.barePath, "worktree", "remove", mergePath)
}

func (x testRepository) assertBranchMissing(t *testing.T, branch string) {
	t.Helper()
	command := gitCommand(t, x.barePath, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	err := command.Run()
	var exitError *exec.ExitError
	require.ErrorAs(t, err, &exitError)
	require.Equal(t, 1, exitError.ExitCode())
}

func (x testRepository) assertPathPresent(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	require.NoError(t, err)
}

func (x testRepository) assertPathMissing(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func at(repo, name string) string {
	return name + "@" + repo
}

func runGitCommand(t *testing.T, directory string, args ...string) string {
	t.Helper()
	output, err := gitCommand(t, directory, args...).CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, output)
	return string(output)
}

func gitCommand(t *testing.T, directory string, args ...string) *exec.Cmd {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", args...)
	command.Dir = directory
	command.Env = append(gitTestEnv(),
		"HOME="+directory,
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	return command
}

// Do not inherit Git overrides, user configuration, shell startup hooks, or
// Timber/Herdr state from the process that runs the suite.
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

func resolvedTempDir(t *testing.T) string {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return directory
}
