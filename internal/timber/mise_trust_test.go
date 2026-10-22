package timber

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAutomaticallyTrustsOnlyEligibleMiseConfigs(t *testing.T) {
	parent := t
	var timberBin string
	for _, test := range []struct {
		name            string
		remote          string
		enabled         bool
		miseFile        bool
		cache           string
		miseFails       bool
		localStart      bool
		symlink         bool
		wantTrust       bool
		wantWarn        string
		interactive     bool
		answer          string
		wantPrompt      bool
		explicitDisable bool
	}{
		{name: "prompt yes", remote: "git@github.com:alice/project.git", miseFile: true, interactive: true, answer: "yes", wantPrompt: true, wantTrust: true},
		{name: "prompt no", remote: "git@github.com:alice/project.git", miseFile: true, interactive: true, answer: "no", wantPrompt: true},
		{name: "prompt default no", remote: "git@github.com:alice/project.git", miseFile: true, interactive: true, wantPrompt: true},
		{name: "explicit refusal", remote: "git@github.com:alice/project.git", miseFile: true, interactive: true, explicitDisable: true},
		{name: "no prompt for other owner", remote: "git@github.com:stranger/project.git", miseFile: true, interactive: true},
		{name: "no prompt without mise", remote: "git@github.com:alice/project.git", interactive: true},
		{name: "refresh on creation", remote: "git@github.com:alice/project.git", enabled: true, miseFile: true, cache: "refresh", wantTrust: true},
		{name: "local branch tracking source", remote: "git@github.com:alice/project.git", enabled: true, miseFile: true, localStart: true, wantTrust: true},
		{name: "symlink config", remote: "git@github.com:alice/project.git", enabled: true, miseFile: true, symlink: true},
		{name: "user SSH", remote: "git@github.com:alice/project.git", enabled: true, miseFile: true, wantTrust: true},
		{name: "organization HTTPS", remote: "https://github.com/SECOND-ORG/project.git", enabled: true, miseFile: true, wantTrust: true},
		{name: "SSH URL", remote: "ssh://git@ssh.github.com:443/alice/project.git", enabled: true, miseFile: true, wantTrust: true},
		{name: "disabled", remote: "git@github.com:alice/project.git", miseFile: true},
		{name: "no mise file", remote: "git@github.com:alice/project.git", enabled: true},
		{name: "unrelated owner", remote: "git@github.com:stranger/project.git", enabled: true, miseFile: true},
		{name: "other host", remote: "git@gitlab.com:alice/project.git", enabled: true, miseFile: true, cache: "missing"},
		{name: "hostname suffix", remote: "https://github.com.evil.test/alice/project", enabled: true, miseFile: true},
		{name: "hostname prefix", remote: "https://evilgithub.com/alice/project", enabled: true, miseFile: true},
		{name: "file URL", remote: "file://github.com/alice/project", enabled: true, miseFile: true},
		{name: "local path", remote: "/github.com/alice/project", enabled: true, miseFile: true},
		{name: "unowned cache missing", remote: "git@github.com:stranger/project", enabled: true, miseFile: true, cache: "missing", wantWarn: "GitHub identity cache is unavailable"},
		{name: "missing cache", remote: "git@github.com:alice/project", enabled: true, miseFile: true, cache: "missing", wantWarn: "GitHub identity cache is unavailable"},
		{name: "corrupt cache", remote: "git@github.com:alice/project", enabled: true, miseFile: true, cache: "corrupt", wantWarn: "GitHub identity cache is unavailable"},
		{name: "expired cache", remote: "git@github.com:alice/project", enabled: true, miseFile: true, cache: "expired", wantWarn: "GitHub identity cache is unavailable"},
		{name: "mise failure", remote: "git@github.com:alice/project", enabled: true, miseFile: true, miseFails: true, wantTrust: true, wantWarn: "mise trust failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.interactive {
				skipIfNoPty(t)
				if timberBin == "" {
					timberBin = buildTestTimberBinary(parent)
				}
			}
			repository := newTestRepository(t)
			if test.miseFile {
				seed := filepath.Join(resolvedTempDir(t), "seed")
				runGitCommand(t, repository.home, "clone", repository.remotePath, seed)
				configureGitUser(t, seed)
				if test.symlink {
					require.NoError(t, os.Symlink("README.md", filepath.Join(seed, "mise.toml")))
				} else {
					require.NoError(t, os.WriteFile(filepath.Join(seed, "mise.toml"), []byte("[tools]\n"), 0o600))
				}
				runGitCommand(t, seed, "add", "mise.toml")
				runGitCommand(t, seed, "commit", "-m", "add mise config")
				runGitCommand(t, seed, "push")
			}
			// Fetch locally. The selected upstream belongs to a different remote,
			// so trusting origin by mistake would not pass the positive cases.
			runGitCommand(t, repository.barePath, "fetch", "origin")
			runGitCommand(t, repository.barePath, "remote", "add", "source", test.remote)
			runGitCommand(t, repository.barePath, "update-ref", "refs/remotes/source/main", "refs/remotes/origin/main")
			runtime := repository.runtime
			runtime.GhExecutable = filepath.Join(resolvedTempDir(t), "missing-gh")
			if test.enabled {
				result := runTimberCommandWithRuntime(t, runtime, "config", "--auto-mise-trust")
				require.NoError(t, result.err)
			}
			if test.explicitDisable {
				result := runTimberCommandWithRuntime(t, runtime, "config", "--no-auto-mise-trust")
				require.NoError(t, result.err)
			}
			if test.cache == "refresh" {
				runtime = installIdentityGh(t, withTestEnvironment(runtime, "GH_TEST_LOG="+filepath.Join(resolvedTempDir(t), "gh.log")))
			}
			if test.cache != "missing" && test.cache != "refresh" {
				fetchedAt := time.Now().UTC()
				if test.cache == "expired" {
					fetchedAt = fetchedAt.Add(-25 * time.Hour)
				}
				cache := fmt.Sprintf(`{"username":"alice","organizations":["first-org","second-org"],"fetched_at":%q}`, fetchedAt.Format(time.RFC3339Nano))
				if test.cache == "corrupt" {
					cache = "{"
				}
				require.NoError(t, writePrivateFile(filepath.Join(runtime.DataHome, "timber", "github-identity.json"), []byte(cache)))
			}
			bin := resolvedTempDir(t)
			logPath := filepath.Join(bin, "mise.log")
			script := fmt.Sprintf("#!/bin/sh\n[ -f .git ] && [ -f mise.toml ] || exit 2\nprintf '%%s\\n' \"$PWD\" \"$@\" > %q\n", logPath)
			if test.miseFails {
				script += "echo 'trust refused' >&2\nexit 1\n"
			}
			require.NoError(t, os.WriteFile(filepath.Join(bin, "mise"), []byte(script), 0o700))
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			runtime = withTestEnvironment(runtime, "PATH="+os.Getenv("PATH"))
			upstream := "source/main"
			if test.localStart {
				runGitCommand(t, repository.barePath, "branch", "--track", "local-start", "source/main")
				upstream = "local-start"
			}
			var result commandResult
			if test.interactive {
				result = interactiveMiseCreate(t, runtime, timberBin, test.answer, "feature")
			} else {
				result = runTimberCommandWithRuntime(t, runtime, "create", "--upstream", upstream, at(testRepoName, "feature"))
			}
			require.NoError(t, result.err)
			assert.FileExists(t, filepath.Join(repository.worktreePath("feature"), ".git"))
			if test.miseFile {
				assert.FileExists(t, filepath.Join(repository.worktreePath("feature"), "mise.toml"))
			}
			if test.wantWarn != "" {
				assert.Contains(t, result.stderr, "warning: automatic mise trust skipped:")
				assert.Contains(t, result.stderr, test.wantWarn)
			} else {
				assert.NotContains(t, result.stderr, "warning:")
			}
			if test.wantTrust {
				contents, err := os.ReadFile(logPath)
				require.NoError(t, err)
				path := repository.worktreePath("feature")
				assert.Equal(t, path+"\ntrust\n"+filepath.Join(path, "mise.toml")+"\n", string(contents))
			} else {
				assert.NoFileExists(t, logPath)
			}
			if test.wantPrompt {
				assert.Contains(t, result.stderr, "permits code execution")
				assert.Contains(t, result.stderr, "[y/N]")
				settings, err := runtime.loadConfig()
				require.NoError(t, err)
				assert.True(t, settings.AutoMiseTrustDecided)
				assert.Equal(t, test.wantTrust, settings.AutoMiseTrust)
				if !test.wantTrust {
					assert.Contains(t, result.stderr, "Enable it later with: timber config --auto-mise-trust")
				}
				again := interactiveMiseCreate(t, runtime, timberBin, test.answer, "second")
				require.NoError(t, again.err)
				assert.NotContains(t, again.stderr, "[y/N]")
				if !test.wantTrust {
					assert.NoFileExists(t, logPath)
					// The documented command can reverse a saved refusal.
					require.NoError(t, runTimberCommandWithRuntime(t, runtime, "config", "--auto-mise-trust").err)
					require.NoError(t, runTimberCommandWithRuntime(t, runtime, "create", "--upstream", "source/main", at(testRepoName, "third")).err)
					assert.FileExists(t, logPath)
				}
			} else {
				assert.NotContains(t, result.stderr, "[y/N]")
			}
		})
	}
}

// Exercise terminal detection and the persisted choice through the real CLI.
func interactiveMiseCreate(t *testing.T, runtime Runtime, bin, answer, branch string) commandResult {
	t.Helper()
	script := `import os, pty, select, signal, sys, time
pid, fd = pty.fork()
if pid == 0:
    os.chdir(sys.argv[2])
    os.execv(sys.argv[1], [sys.argv[1], "create", "--upstream", "source/main", sys.argv[4] + "@repo"])
output = b""
sent = False
try:
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        ready, _, _ = select.select([fd], [], [], 0.1)
        if not ready:
            continue
        try:
            chunk = os.read(fd, 8192)
        except OSError:
            break
        if not chunk:
            break
        output += chunk
        if b"[y/N] " in output and not sent:
            os.write(fd, (sys.argv[3] + "\n").encode())
            sent = True
    else:
        raise RuntimeError("timed out waiting for create: " + output.decode(errors="replace"))
    _, status = os.waitpid(pid, 0)
    sys.stdout.write(output.decode(errors="replace"))
    sys.exit(os.waitstatus_to_exitcode(status))
finally:
    os.close(fd)
    try:
        os.kill(pid, signal.SIGKILL)
    except ProcessLookupError:
        pass
`
	command := testCommand(t, "python3", "-c", script, filepath.Join(bin, "timber"), runtime.HomeDirectory, answer, branch)
	command.Env = runtime.Environment
	output, err := command.CombinedOutput()
	return commandResult{stderr: string(output), err: err}
}
