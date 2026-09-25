package timber

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateZshGeneratesWrapperCompletionAndAutoloadHelper(t *testing.T) {
	t.Parallel()

	outDir := resolvedTempDir(t)
	result := runTimberCommand(t, "generate", "zsh", "--out", outDir, "--force")
	require.NoError(t, result.err, result.stderr)

	autoloadPath := filepath.Join(outDir, "_t_autoload")
	autoloadContents, err := os.ReadFile(autoloadPath)
	require.NoError(t, err)
	assert.Equal(t, "#autoload t", strings.SplitN(string(autoloadContents), "\n", 2)[0])

	completionPath := filepath.Join(outDir, "_t")
	completionContents, err := os.ReadFile(completionPath)
	require.NoError(t, err)
	assert.Equal(t, "#compdef t", strings.SplitN(string(completionContents), "\n", 2)[0])
	assert.NotContains(t, string(completionContents), "_foo()")
	assert.Contains(t, string(completionContents), "_t()\n{")

	functionPath := filepath.Join(outDir, "t")
	functionContents, err := os.ReadFile(functionPath)
	require.NoError(t, err)
	assert.NotContains(t, string(functionContents), "foo() {")
	assert.Contains(t, string(functionContents), "t() {")
}

func TestGeneratedZshCompletionHasValidSyntax(t *testing.T) {
	t.Parallel()
	zshPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is not installed")
	}

	outDir := resolvedTempDir(t)
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir).err)

	output, err := testCommand(t, zshPath, "-n", filepath.Join(outDir, "_t")).CombinedOutput()
	require.NoError(t, err, string(output))
}

func TestGeneratedCompletionDelegatesToTimber(t *testing.T) {
	t.Parallel()

	outDir := resolvedTempDir(t)
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir).err)

	completionContents, err := os.ReadFile(filepath.Join(outDir, "_t"))
	require.NoError(t, err)
	completion := string(completionContents)
	// Flags and arguments come from 'timber __complete' so they cannot
	// drift from the Cobra definitions; only wrapper-only flags are
	// layered on top.
	assert.Contains(t, completion, "command timber __complete")
	assert.Contains(t, completion, "--no-cd:Create without changing directories")
	assert.NotContains(t, completion, "->worktrees")
	assert.NotContains(t, completion, "->repo_qualifiers")
}

func TestListFlagCompletionOffersPrJsonSort(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	for _, command := range []string{"list", "ls"} {
		stdout := runCompleteWithRuntime(t, testRepository.runtime, command, "--")
		assert.Contains(t, stdout, "--pr", "command=%s", command)
		assert.Contains(t, stdout, "--json", "command=%s", command)
		assert.Contains(t, stdout, "--sort", "command=%s", command)
	}
}

func TestListSortCompletionOffersModes(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	stdout := runCompleteWithRuntime(t, testRepository.runtime, "list", "--sort", "")
	assert.Contains(t, stdout, "recency")
	assert.Contains(t, stdout, "repo")
	assert.Contains(t, stdout, "worktree")
}

func TestGenerateZshUsesCustomWrapperName(t *testing.T) {
	t.Parallel()

	outDir := resolvedTempDir(t)
	result := runTimberCommand(t, "generate", "zsh", "--name", "foo", "--out", outDir)
	require.NoError(t, result.err, result.stderr)

	for _, defaultPath := range []string{"t", "_t", "_t_autoload"} {
		_, err := os.Stat(filepath.Join(outDir, defaultPath))
		require.ErrorIs(t, err, os.ErrNotExist, defaultPath)
	}

	autoloadContents, err := os.ReadFile(filepath.Join(outDir, "_foo_autoload"))
	require.NoError(t, err)
	assert.Equal(t, "#autoload foo", strings.SplitN(string(autoloadContents), "\n", 2)[0])

	completionContents, err := os.ReadFile(filepath.Join(outDir, "_foo"))
	require.NoError(t, err)
	assert.Equal(t, "#compdef foo", strings.SplitN(string(completionContents), "\n", 2)[0])
	assert.Contains(t, string(completionContents), "_foo()\n{")
	assert.NotContains(t, string(completionContents), "_t()")

	functionContents, err := os.ReadFile(filepath.Join(outDir, "foo"))
	require.NoError(t, err)
	assert.Contains(t, string(functionContents), "foo() {")
	assert.NotContains(t, string(functionContents), "t() {")
}

// buildTestTimberBinary builds the current module into a temporary bin
// directory and returns it, so pty completion tests exercise the same
// binary the generated completion delegates to via 'timber __complete'.
func buildTestTimberBinary(t *testing.T) string {
	t.Helper()
	_, callerFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Join(filepath.Dir(callerFile), "..", "..")
	binDir := resolvedTempDir(t)

	goEnv := testCommand(t, "go", "env", "GOMODCACHE", "GOCACHE", "GOPROXY", "GOTOOLCHAIN")
	goEnv.Dir = repoRoot
	goEnv.Env = os.Environ()
	envOutput, err := goEnv.Output()
	require.NoError(t, err)
	envKeys := []string{"GOMODCACHE", "GOCACHE", "GOPROXY", "GOTOOLCHAIN"}

	build := testCommand(t, "go", "build", "-o", filepath.Join(binDir, "timber"), ".")
	build.Dir = repoRoot
	// testCommand isolates HOME, which would empty the module and build
	// caches; keep the real Go toolchain directories so the build stays
	// offline and fast.
	for index, line := range strings.Split(strings.TrimSpace(string(envOutput)), "\n") {
		if index < len(envKeys) && line != "" {
			build.Env = replaceTestEnvironment(build.Env, envKeys[index]+"="+line)
		}
	}
	buildOutput, err := build.CombinedOutput()
	require.NoError(t, err, string(buildOutput))
	return binDir
}

func TestGeneratedCreateCompletesUniqueRepoPrefix(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	skipIfNoPty(t)

	home := resolvedTempDir(t)
	dataHome := filepath.Join(home, ".local", "share")
	worktreeRoot := filepath.Join(home, "worktrees")
	require.NoError(t, os.MkdirAll(filepath.Join(dataHome, "timber", "repos", "timber.git"), 0o755))

	outDir := resolvedTempDir(t)
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir, "--force").err)

	scriptPath := filepath.Join(resolvedTempDir(t), "complete.py")
	script := `import os, pty, select, time, sys

compdir = sys.argv[1]
zdot = sys.argv[2]
os.makedirs(zdot, exist_ok=True)
open(os.path.join(zdot, ".zshrc"), "w").write("")
os.environ["ZDOTDIR"] = zdot
os.environ["HOME"] = sys.argv[3]
os.environ["XDG_DATA_HOME"] = sys.argv[4]
os.environ["TIMBER_WORKTREE_ROOT"] = sys.argv[5]
os.environ["PATH"] = sys.argv[6] + ":" + os.environ["PATH"]

pid, fd = pty.fork()
if pid == 0:
    os.execvp("zsh", ["zsh", "-f", "-i"])

def recv(timeout=1.0):
    buf = b""
    end = time.time() + timeout
    while time.time() < end:
        ready, _, _ = select.select([fd], [], [], max(0.05, end - time.time()))
        if not ready:
            continue
        try:
            chunk = os.read(fd, 8192)
        except OSError:
            break
        if not chunk:
            break
        buf += chunk
        end = time.time() + 0.2
    return buf

def send(data):
    os.write(fd, data.encode() if isinstance(data, str) else data)

def wait_compinit(timeout=30.0):
    out = b""
    end = time.time() + timeout
    while time.time() < end and b"COMPINIT_DONE" not in out:
        out += recv(1.0)
    return out

def wait_expansion(timeout=10.0):
    out = b""
    end = time.time() + timeout
    while time.time() < end and b"feature/login" not in out and b"@timber" not in out:
        out += recv(0.5)
    return out

recv(0.3)
log = b""
send("fpath=(" + compdir + " $fpath); autoload -Uz compinit; compinit -u -D; echo COMPINIT_DONE:$SECONDS\n")
log += wait_compinit()
send("(( $+_comps[t] )) && echo HAVE_T_COMP || echo NO_T_COMP\n")
log += recv(1.0)
send("zstyle ':completion:*' matcher-list 'm:{a-zA-Z}={A-Za-z}' 'r:|[._-]=* r:|=*' 'l:|=* r:|=*'\n")
recv(0.2)
send("\x15")
recv(0.1)
send("t create @t")
time.sleep(0.05)
send("\t")
log += wait_expansion()
send("exit\n")
recv(0.2)
sys.stdout.write(log.decode("utf-8", "replace"))
`
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o644))

	timberBin := buildTestTimberBinary(t)

	command := testCommand(t,
		"python3",
		scriptPath,
		outDir,
		filepath.Join(resolvedTempDir(t), "zdot"),
		home,
		dataHome,
		worktreeRoot,
		timberBin,
	)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Contains(t, string(output), "t create @timber")
}

func TestGeneratedSwitchCompletesWorktreeNamesAcrossRepos(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	skipIfNoPty(t)

	home := resolvedTempDir(t)
	dataHome := filepath.Join(home, ".local", "share")
	worktreeRoot := filepath.Join(home, "worktrees")
	require.NoError(t, os.MkdirAll(filepath.Join(dataHome, "timber", "repos", "timber.git"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dataHome, "timber", "repos", "other.git"), 0o755))
	makeWorktree := func(repoName, worktreeName string) {
		t.Helper()
		worktreePath := filepath.Join(worktreeRoot, repoName, worktreeName, repoName)
		require.NoError(t, os.MkdirAll(worktreePath, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(worktreePath, ".git"), nil, 0o644))
	}
	makeWorktree("timber", "feature/login")
	makeWorktree("other", "feature/api")

	outDir := resolvedTempDir(t)
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir, "--force").err)

	scriptPath := filepath.Join(resolvedTempDir(t), "complete.py")
	script := `import os, pty, select, time, sys

compdir, zdot, home, data_home, worktree_root, line, bindir = sys.argv[1:8]
os.makedirs(zdot, exist_ok=True)
open(os.path.join(zdot, ".zshrc"), "w").write("")
os.environ["ZDOTDIR"] = zdot
os.environ["HOME"] = home
os.environ["XDG_DATA_HOME"] = data_home
os.environ["TIMBER_WORKTREE_ROOT"] = worktree_root
os.environ["PATH"] = bindir + ":" + os.environ["PATH"]

pid, fd = pty.fork()
if pid == 0:
    os.execvp("zsh", ["zsh", "-f", "-i"])

def recv(timeout=1.0):
    buf = b""
    end = time.time() + timeout
    while time.time() < end:
        ready, _, _ = select.select([fd], [], [], max(0.05, end - time.time()))
        if not ready:
            continue
        try:
            chunk = os.read(fd, 8192)
        except OSError:
            break
        if not chunk:
            break
        buf += chunk
        end = time.time() + 0.2
    return buf

def send(data):
    os.write(fd, data.encode() if isinstance(data, str) else data)

def wait_compinit(timeout=30.0):
    out = b""
    end = time.time() + timeout
    while time.time() < end and b"COMPINIT_DONE" not in out:
        out += recv(1.0)
    return out

def wait_expansion(timeout=10.0):
    out = b""
    end = time.time() + timeout
    while time.time() < end and b"feature/login" not in out and b"@timber" not in out:
        out += recv(0.5)
    return out

recv(0.3)
log = b""
send("cd " + home + "\n")
recv(0.2)
send("fpath=(" + compdir + " $fpath); autoload -Uz compinit; compinit -u -D; echo COMPINIT_DONE:$SECONDS\n")
log += wait_compinit()
send("(( $+_comps[t] )) && echo HAVE_T_COMP || echo NO_T_COMP\n")
log += recv(1.0)
send("\x15")
recv(0.1)
send(line)
time.sleep(0.05)
send("\t")
log += wait_expansion()
send("exit\n")
recv(0.2)
sys.stdout.write(log.decode("utf-8", "replace"))
`
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o644))

	timberBin := buildTestTimberBinary(t)

	runComplete := func(line string) string {
		t.Helper()
		command := testCommand(t,
			"python3",
			scriptPath,
			outDir,
			filepath.Join(resolvedTempDir(t), "zdot"),
			home,
			dataHome,
			worktreeRoot,
			line,
			timberBin,
		)
		output, err := command.CombinedOutput()
		require.NoError(t, err, string(output))
		return string(output)
	}

	unique := runComplete("t switch feature/l")
	assert.Contains(t, unique, "t switch feature/login")
	assert.NotContains(t, unique, "feature/login@")

	uniqueAlias := runComplete("t sw feature/l")
	assert.Contains(t, uniqueAlias, "t sw feature/login")
	assert.NotContains(t, uniqueAlias, "feature/login@")

	makeWorktree("other", "feature/login")
	ambiguous := runComplete("t switch feature/l")
	assert.Contains(t, ambiguous, "t switch feature/login@")
}

// TestGeneratedDelegatedCompletionEndToEnd proves the generated completion
// delegates to 'timber __complete': list flags, sort values, and the
// wrapper-only create --no-cd flag all complete in a real zsh session.
func TestGeneratedDelegatedCompletionEndToEnd(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	skipIfNoPty(t)

	home := resolvedTempDir(t)
	dataHome := filepath.Join(home, ".local", "share")
	worktreeRoot := filepath.Join(home, "worktrees")

	outDir := resolvedTempDir(t)
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir, "--force").err)
	timberBin := buildTestTimberBinary(t)

	scriptPath := filepath.Join(resolvedTempDir(t), "complete.py")
	script := `import os, pty, select, time, sys

compdir, zdot, home, data_home, worktree_root, bindir, line = sys.argv[1:8]
wants = sys.argv[8:]
os.makedirs(zdot, exist_ok=True)
open(os.path.join(zdot, ".zshrc"), "w").write("")
os.environ["ZDOTDIR"] = zdot
os.environ["HOME"] = home
os.environ["XDG_DATA_HOME"] = data_home
os.environ["TIMBER_WORKTREE_ROOT"] = worktree_root
os.environ["PATH"] = bindir + ":" + os.environ["PATH"]

pid, fd = pty.fork()
if pid == 0:
    os.execvp("zsh", ["zsh", "-f", "-i"])

def recv(timeout=1.0):
    buf = b""
    end = time.time() + timeout
    while time.time() < end:
        ready, _, _ = select.select([fd], [], [], max(0.05, end - time.time()))
        if not ready:
            continue
        try:
            chunk = os.read(fd, 8192)
        except OSError:
            break
        if not chunk:
            break
        buf += chunk
        end = time.time() + 0.2
    return buf

def send(data):
    os.write(fd, data.encode() if isinstance(data, str) else data)

def wait_compinit(timeout=30.0):
    out = b""
    end = time.time() + timeout
    while time.time() < end and b"COMPINIT_DONE" not in out:
        out += recv(1.0)
    return out

def wait_expansion(timeout=10.0):
    out = b""
    end = time.time() + timeout
    want = [w.encode() for w in wants]
    while time.time() < end and not all(w in out for w in want):
        out += recv(0.5)
    return out

recv(0.3)
log = b""
send("fpath=(" + compdir + " $fpath); autoload -Uz compinit; compinit -u -D; echo COMPINIT_DONE:$SECONDS\n")
log += wait_compinit()
send("(( $+_comps[t] )) && echo HAVE_T_COMP || echo NO_T_COMP\n")
log += recv(1.0)
send("\x15")
recv(0.1)
send(line)
time.sleep(0.05)
send("\t")
log += wait_expansion()
send("exit\n")
recv(0.2)
sys.stdout.write(log.decode("utf-8", "replace"))
`
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o644))

	runComplete := func(line string, wants ...string) string {
		t.Helper()
		args := append([]string{
			"python3",
			scriptPath,
			outDir,
			filepath.Join(resolvedTempDir(t), "zdot"),
			home,
			dataHome,
			worktreeRoot,
			timberBin,
			line,
		}, wants...)
		command := testCommand(t, args[0], args[1:]...)
		output, err := command.CombinedOutput()
		require.NoError(t, err, string(output))
		return string(output)
	}

	flags := runComplete("t ls --", "--sort", "--pr", "--json")
	assert.Contains(t, flags, "--sort")
	assert.Contains(t, flags, "--pr")
	assert.Contains(t, flags, "--json")

	sortValue := runComplete("t list --sort rec", "recency")
	assert.Contains(t, sortValue, "t list --sort recency")

	wrapperFlags := runComplete("t create --no-", "--no-cd", "--no-herdr")
	assert.Contains(t, wrapperFlags, "--no-cd")
	assert.Contains(t, wrapperFlags, "--no-herdr")
}

func TestGeneratedZshWrapperAutoloadsAfterCompinit(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}

	outDir := resolvedTempDir(t)
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir).err)

	binDir := resolvedTempDir(t)
	fakeTimber := `#!/bin/sh
printf '%s\n' "$@"
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "timber"), []byte(fakeTimber), 0o755))

	command := testCommand(t,
		"zsh", "-f", "-c",
		`fpath=("$1" $fpath)
autoload -Uz compinit
compinit -u -D 2>/dev/null
t list`,
		"--", outDir,
	)
	// -u keeps compinit from aborting on insecure directories (as on CI),
	// and PATH limited to the fake timber keeps an installed t from
	// masking a broken autoload.
	command.Env = replaceTestEnvironment(command.Env, "PATH="+binDir)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Equal(t, "list", strings.TrimSpace(string(output)))
}

func TestGeneratedZshWrapperChangesToRenamedCurrentWorktree(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}

	outDir := resolvedTempDir(t)
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir, "--force").err)

	worktreeParent := resolvedTempDir(t)
	oldWorktree := filepath.Join(worktreeParent, "old")
	oldSubdirectory := filepath.Join(oldWorktree, "nested")
	require.NoError(t, os.MkdirAll(oldSubdirectory, 0o755))

	binDir := resolvedTempDir(t)
	fakeTimber := `#!/bin/sh
old_worktree=$(dirname "$PWD")
new_worktree=$(dirname "$old_worktree")/new
mv "$old_worktree" "$new_worktree" || exit $?
printf '%s\n' "$new_worktree/nested" > "$TIMBER_RENAME_PATH_FILE"
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "timber"), []byte(fakeTimber), 0o755))

	command := testCommand(t,
		"zsh", "-f", "-c",
		`source "$1"; cd "$2"; t repo rename old new >/dev/null; pwd -P`,
		"--", filepath.Join(outDir, "t"), oldSubdirectory,
	)
	command.Env = replaceTestEnvironment(command.Env, "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Equal(t, canonicalPath(filepath.Join(worktreeParent, "new", "nested")), strings.TrimSpace(string(output)))
}

func TestGeneratedZshWrapperRestoresDirectoryOnFailure(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}

	outDir := resolvedTempDir(t)
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir, "--force").err)

	startDir := resolvedTempDir(t)
	binDir := resolvedTempDir(t)
	fakeTimber := `#!/bin/sh
exit 17
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "timber"), []byte(fakeTimber), 0o755))

	command := testCommand(t,
		"zsh", "-f", "-c",
		`source "$1"; cd "$2"; t remove feature@repo >/dev/null; exit_status=$?; printf '%s %s\n' "$exit_status" "$PWD"`,
		"--", filepath.Join(outDir, "t"), startDir,
	)
	command.Env = replaceTestEnvironment(command.Env, "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Equal(t, fmt.Sprintf("17 %s", canonicalPath(startDir)), strings.TrimSpace(string(output)))
}

func TestGeneratedZshWrapperChangesDirectoryOnSwitch(t *testing.T) {
	t.Parallel()
	targetDir := resolvedTempDir(t)
	fakeTimber := `#!/bin/sh
printf '%s\n' "$TIMBER_SWITCH_PATH_FILE_TARGET" > "$TIMBER_SWITCH_PATH_FILE"
`
	command := generatedZshCommand(t, fakeTimber,
		`source "$1"; t switch feature@repo >/dev/null; pwd -P`,
	)
	command.Env = replaceTestEnvironment(command.Env, "TIMBER_SWITCH_PATH_FILE_TARGET="+targetDir)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Equal(t, canonicalPath(targetDir), strings.TrimSpace(string(output)))
}

func TestGeneratedZshWrapperLeavesImportedSourceDirectory(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}

	outDir := resolvedTempDir(t)
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir, "--force").err)

	startDir := resolvedTempDir(t)
	sourceDir := resolvedTempDir(t)
	binDir := resolvedTempDir(t)
	fakeTimber := `#!/bin/sh
rm -rf "$3"
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "timber"), []byte(fakeTimber), 0o755))

	command := testCommand(t,
		"zsh", "-f", "-c",
		`source "$1"; cd "$2"; t repo import "$3" >/dev/null; printf '%s' "$PWD"`,
		"--", filepath.Join(outDir, "t"), startDir, sourceDir,
	)
	command.Env = replaceTestEnvironment(command.Env, "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Equal(t, canonicalPath(command.Dir), strings.TrimSpace(string(output)))
}

func TestGeneratedZshWrapperRestoresDirectoryOnFailedImport(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}

	outDir := resolvedTempDir(t)
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir, "--force").err)

	startDir := resolvedTempDir(t)
	sourceDir := resolvedTempDir(t)
	binDir := resolvedTempDir(t)
	fakeTimber := `#!/bin/sh
exit 17
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "timber"), []byte(fakeTimber), 0o755))

	command := testCommand(t,
		"zsh", "-f", "-c",
		`source "$1"; cd "$2"; t repo import "$3" >/dev/null; exit_status=$?; printf '%s %s' "$exit_status" "$PWD"`,
		"--", filepath.Join(outDir, "t"), startDir, sourceDir,
	)
	command.Env = replaceTestEnvironment(command.Env, "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Equal(t, fmt.Sprintf("17 %s", canonicalPath(startDir)), strings.TrimSpace(string(output)))
}

func TestGenerateZshRefusesOverwriteWithoutForce(t *testing.T) {
	t.Parallel()
	outDir := resolvedTempDir(t)
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir).err)
	result := runTimberCommand(t, "generate", "zsh", "--out", outDir)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "already exists")
}

func TestGenerateZshChecksAutoloadHelperCollisionBeforeWriting(t *testing.T) {
	t.Parallel()
	outDir := resolvedTempDir(t)
	autoloadPath := filepath.Join(outDir, "_t_autoload")
	require.NoError(t, os.WriteFile(autoloadPath, []byte("existing helper\n"), 0o644))

	result := runTimberCommand(t, "generate", "zsh", "--out", outDir)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "autoload helper file")

	autoloadContents, err := os.ReadFile(autoloadPath)
	require.NoError(t, err)
	assert.Equal(t, "existing helper\n", string(autoloadContents))
	for _, untouchedPath := range []string{"t", "_t"} {
		_, err := os.Stat(filepath.Join(outDir, untouchedPath))
		require.ErrorIs(t, err, os.ErrNotExist, untouchedPath)
	}

	forceResult := runTimberCommand(t, "generate", "zsh", "--out", outDir, "--force")
	require.NoError(t, forceResult.err, forceResult.stderr)
	autoloadContents, err = os.ReadFile(autoloadPath)
	require.NoError(t, err)
	assert.Equal(t, "#autoload t", strings.SplitN(string(autoloadContents), "\n", 2)[0])
}
