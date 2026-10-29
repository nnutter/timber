package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAutomaticallyInstallsHKHooksOnlyForEligibleRepositories(t *testing.T) {
	parent := t
	var timberBin string
	for _, test := range []struct {
		name            string
		remote          string
		enabled         bool
		hkFile          bool
		miseFile        bool
		miseSymlink     bool
		hkSymlink       bool
		cache           string
		hkFails         bool
		localStart      bool
		wantInstall     bool
		wantMise        bool
		wantWarn        string
		interactive     bool
		answer          string
		wantPrompt      bool
		explicitDisable bool
		noOrganizations bool
	}{
		{name: "install with mise", remote: "git@github.com:alice/project.git", enabled: true, hkFile: true, miseFile: true, wantInstall: true, wantMise: true},
		{name: "install without mise", remote: "git@github.com:alice/project.git", enabled: true, hkFile: true, wantInstall: true},
		{name: "mise symlink drops option", remote: "git@github.com:alice/project.git", enabled: true, hkFile: true, miseFile: true, miseSymlink: true, wantInstall: true},
		{name: "prompt yes", remote: "git@github.com:alice/project.git", hkFile: true, miseFile: true, interactive: true, answer: "yes", wantPrompt: true, wantInstall: true, wantMise: true},
		{name: "prompt yes without mise", remote: "git@github.com:alice/project.git", hkFile: true, interactive: true, answer: "yes", wantPrompt: true, wantInstall: true},
		{name: "prompt without organizations", remote: "git@github.com:alice/project.git", hkFile: true, interactive: true, answer: "yes", wantPrompt: true, wantInstall: true, noOrganizations: true},
		{name: "prompt no", remote: "git@github.com:alice/project.git", hkFile: true, miseFile: true, interactive: true, answer: "no", wantPrompt: true},
		{name: "prompt default no", remote: "git@github.com:alice/project.git", hkFile: true, interactive: true, wantPrompt: true},
		{name: "explicit refusal", remote: "git@github.com:alice/project.git", hkFile: true, interactive: true, explicitDisable: true},
		{name: "no prompt for other owner", remote: "git@github.com:stranger/project.git", hkFile: true, interactive: true},
		{name: "no prompt without hk", remote: "git@github.com:alice/project.git", miseFile: true, interactive: true},
		{name: "disabled", remote: "git@github.com:alice/project.git", hkFile: true, miseFile: true},
		{name: "no hk file", remote: "git@github.com:alice/project.git", enabled: true, miseFile: true},
		{name: "symlink hk config", remote: "git@github.com:alice/project.git", enabled: true, hkFile: true, hkSymlink: true},
		{name: "unrelated owner", remote: "git@github.com:stranger/project.git", enabled: true, hkFile: true},
		{name: "other host", remote: "git@gitlab.com:alice/project.git", enabled: true, hkFile: true, cache: "missing"},
		{name: "local branch tracking source", remote: "git@github.com:alice/project.git", enabled: true, hkFile: true, localStart: true, wantInstall: true},
		{name: "organization HTTPS", remote: "https://github.com/SECOND-ORG/project.git", enabled: true, hkFile: true, wantInstall: true},
		{name: "missing cache", remote: "git@github.com:alice/project.git", enabled: true, hkFile: true, cache: "missing", wantWarn: "GitHub identity cache is unavailable"},
		{name: "expired cache", remote: "git@github.com:alice/project.git", enabled: true, hkFile: true, cache: "expired", wantWarn: "GitHub identity cache is unavailable"},
		{name: "hk failure", remote: "git@github.com:alice/project.git", enabled: true, hkFile: true, hkFails: true, wantInstall: true, wantWarn: "hk install failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.interactive {
				shellSkipIfNoPty(t)
				if timberBin == "" {
					timberBin = buildTestTimberBinary(parent)
				}
			}
			repository := newTestRepository(t)
			if test.hkFile || test.miseFile {
				seedRepositoryFiles(t, repository, test.hkFile, test.hkSymlink, test.miseFile, test.miseSymlink)
			}
			// Fetch locally. The selected upstream belongs to a different remote,
			// so installing hooks for origin by mistake would not pass the
			// positive cases.
			runGitCommand(t, repository.barePath, "fetch", "origin")
			runGitCommand(t, repository.barePath, "remote", "add", "source", test.remote)
			runGitCommand(t, repository.barePath, "update-ref", "refs/remotes/source/main", "refs/remotes/origin/main")
			runtime := repository.runtime
			runtime.GhExecutable = filepath.Join(resolvedTempDir(t), "missing-gh")
			if test.enabled {
				result := runTimberCommandWithRuntime(t, runtime, "config", "--auto-hk-install")
				require.NoError(t, result.err)
			}
			if test.explicitDisable {
				result := runTimberCommandWithRuntime(t, runtime, "config", "--no-auto-hk-install")
				require.NoError(t, result.err)
			}
			if test.interactive {
				// Keep automatic mise trust from asking its own question first.
				result := runTimberCommandWithRuntime(t, runtime, "config", "--no-auto-mise-trust")
				require.NoError(t, result.err)
			}
			if test.cache != "missing" {
				fetchedAt := time.Now().UTC()
				if test.cache == "expired" {
					fetchedAt = fetchedAt.Add(-25 * time.Hour)
				}
				organizations := `["first-org","second-org"]`
				if test.noOrganizations {
					organizations = `[]`
				}
				cache := fmt.Sprintf(`{"username":"alice","organizations":%s,"fetched_at":%q}`, organizations, fetchedAt.Format(time.RFC3339Nano))
				writeIdentityCache(t, runtime, cache)
			}
			bin := resolvedTempDir(t)
			logPath := filepath.Join(bin, "hk.log")
			script := fmt.Sprintf("#!/bin/sh\n[ -f .git ] && [ -f hk.pkl ] || exit 2\nprintf '%%s\\n' \"$PWD\" \"$@\" > %q\n", logPath)
			if test.hkFails {
				script += "echo 'install refused' >&2\nexit 1\n"
			}
			require.NoError(t, os.WriteFile(filepath.Join(bin, "hk"), []byte(script), 0o700))
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			runtime = withTestEnvironment(runtime, "PATH="+os.Getenv("PATH"))
			upstream := "source/main"
			if test.localStart {
				runGitCommand(t, repository.barePath, "branch", "--track", "local-start", "source/main")
				upstream = "local-start"
			}
			var result commandResult
			if test.interactive {
				result = interactiveCreate(t, runtime, timberBin, test.answer, "feature")
			} else {
				result = runTimberCommandWithRuntime(t, runtime, "create", "--upstream", upstream, at(testRepoName, "feature"))
			}
			require.NoError(t, result.err)
			assert.FileExists(t, filepath.Join(repository.worktreePath("feature"), ".git"))
			if test.wantWarn != "" {
				assert.Contains(t, result.stderr, "warning: automatic hk install skipped:")
				assert.Contains(t, result.stderr, test.wantWarn)
			} else {
				assert.NotContains(t, result.stderr, "warning:")
			}
			if test.wantInstall {
				contents, err := os.ReadFile(logPath)
				require.NoError(t, err)
				path := repository.worktreePath("feature")
				want := path + "\ninstall\n"
				if test.wantMise {
					want += "--mise\n"
				}
				assert.Equal(t, want, string(contents))
			} else {
				assert.NoFileExists(t, logPath)
			}
			if test.wantPrompt {
				assert.Contains(t, result.stderr, "permits code execution")
				assert.Contains(t, result.stderr, "[y/N]")
				assert.Contains(t, result.stderr, "GitHub username: alice")
				organizations := "GitHub organizations: first-org, second-org"
				if test.noOrganizations {
					organizations = "GitHub organizations: none"
				}
				assert.Contains(t, result.stderr, organizations)
				assert.Less(t, strings.Index(result.stderr, organizations), strings.Index(result.stderr, "[y/N]"))
				settings, err := runtime.LoadConfig()
				require.NoError(t, err)
				assert.True(t, settings.AutoHKInstallDecided)
				assert.Equal(t, test.wantInstall, settings.AutoHKInstall)
				if !test.wantInstall {
					assert.Contains(t, result.stderr, "Enable it later with: timber config --auto-hk-install")
				}
				again := interactiveCreate(t, runtime, timberBin, test.answer, "second")
				require.NoError(t, again.err)
				assert.NotContains(t, again.stderr, "[y/N]")
				if !test.wantInstall {
					assert.NoFileExists(t, logPath)
					// The documented command can reverse a saved refusal.
					require.NoError(t, runTimberCommandWithRuntime(t, runtime, "config", "--auto-hk-install").err)
					require.NoError(t, runTimberCommandWithRuntime(t, runtime, "create", "--upstream", "source/main", at(testRepoName, "third")).err)
					assert.FileExists(t, logPath)
				}
			} else {
				assert.NotContains(t, result.stderr, "[y/N]")
			}
		})
	}
}

func TestCreateTrustsMiseBeforeInstallingHKHooks(t *testing.T) {
	repository := newTestRepository(t)
	seedRepositoryFiles(t, repository, true, false, true, false)
	runGitCommand(t, repository.barePath, "fetch", "origin")
	runGitCommand(t, repository.barePath, "remote", "add", "source", "git@github.com:alice/project.git")
	runGitCommand(t, repository.barePath, "update-ref", "refs/remotes/source/main", "refs/remotes/origin/main")

	runtime := repository.runtime
	cache := fmt.Sprintf(`{"username":"alice","organizations":[],"fetched_at":%q}`, time.Now().UTC().Format(time.RFC3339Nano))
	writeIdentityCache(t, runtime, cache)
	require.NoError(t, runTimberCommandWithRuntime(t, runtime, "config", "--auto-mise-trust").err)
	require.NoError(t, runTimberCommandWithRuntime(t, runtime, "config", "--auto-hk-install").err)

	bin := resolvedTempDir(t)
	logPath := filepath.Join(bin, "automation.log")
	for name, operation := range map[string]string{"mise": "mise", "hk": "hk"} {
		script := fmt.Sprintf("#!/bin/sh\nprintf '%s %%s\\n' \"$*\" >> %q\n", operation, logPath)
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700))
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	runtime = withTestEnvironment(runtime, "PATH="+os.Getenv("PATH"))

	result := runTimberCommandWithRuntime(t, runtime, "create", "--upstream", "source/main", at(testRepoName, "feature"))
	require.NoError(t, result.err)
	assert.NotContains(t, result.stderr, "warning:")

	contents, err := os.ReadFile(logPath)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(contents)), "\n")
	require.Len(t, lines, 2)
	assert.True(t, strings.HasPrefix(lines[0], "mise trust "))
	assert.Equal(t, "hk install --mise", lines[1])
}

func seedRepositoryFiles(t *testing.T, repository testRepository, hkFile, hkSymlink, miseFile, miseSymlink bool) {
	t.Helper()
	seed := filepath.Join(resolvedTempDir(t), "seed")
	runGitCommand(t, repository.home, "clone", repository.remotePath, seed)
	runGitCommand(t, seed, "config", "user.name", "Test User")
	runGitCommand(t, seed, "config", "user.email", "test@example.com")
	if hkFile {
		if hkSymlink {
			require.NoError(t, os.Symlink("README.md", filepath.Join(seed, "hk.pkl")))
		} else {
			require.NoError(t, os.WriteFile(filepath.Join(seed, "hk.pkl"), []byte("hooks {}\n"), 0o600))
		}
	}
	if miseFile {
		if miseSymlink {
			require.NoError(t, os.Symlink("README.md", filepath.Join(seed, "mise.toml")))
		} else {
			require.NoError(t, os.WriteFile(filepath.Join(seed, "mise.toml"), []byte("[tools]\n"), 0o600))
		}
	}
	runGitCommand(t, seed, "add", ".")
	runGitCommand(t, seed, "commit", "-m", "add repository config")
	runGitCommand(t, seed, "push")
}
