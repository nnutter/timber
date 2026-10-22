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

func TestCreateAutomaticallyInstallsHKHooksOnlyForEligibleRepositories(t *testing.T) {
	for _, test := range []struct {
		name        string
		remote      string
		enabled     bool
		hkFile      bool
		miseFile    bool
		miseSymlink bool
		hkSymlink   bool
		cache       string
		hkFails     bool
		localStart  bool
		wantInstall bool
		wantMise    bool
		wantWarn    string
	}{
		{name: "install with mise", remote: "git@github.com:alice/project.git", enabled: true, hkFile: true, miseFile: true, wantInstall: true, wantMise: true},
		{name: "install without mise", remote: "git@github.com:alice/project.git", enabled: true, hkFile: true, wantInstall: true},
		{name: "mise symlink drops option", remote: "git@github.com:alice/project.git", enabled: true, hkFile: true, miseFile: true, miseSymlink: true, wantInstall: true},
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
			if test.cache != "missing" {
				fetchedAt := time.Now().UTC()
				if test.cache == "expired" {
					fetchedAt = fetchedAt.Add(-25 * time.Hour)
				}
				cache := fmt.Sprintf(`{"username":"alice","organizations":["first-org","second-org"],"fetched_at":%q}`, fetchedAt.Format(time.RFC3339Nano))
				require.NoError(t, writePrivateFile(filepath.Join(runtime.DataHome, "timber", "github-identity.json"), []byte(cache)))
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
			result := runTimberCommandWithRuntime(t, runtime, "create", "--upstream", upstream, at(testRepoName, "feature"))
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
		})
	}
}

func seedRepositoryFiles(t *testing.T, repository testRepository, hkFile, hkSymlink, miseFile, miseSymlink bool) {
	t.Helper()
	seed := filepath.Join(resolvedTempDir(t), "seed")
	runGitCommand(t, repository.home, "clone", repository.remotePath, seed)
	configureGitUser(t, seed)
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
