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

	"github.com/nnutter/timber/internal/timber"
)

type miseTrustCase struct {
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
	noOrganizations bool
}

func runMiseTrustCase(t *testing.T, parent *testing.T, timberBin *string, test miseTrustCase) {
	t.Helper()
	if test.interactive {
		shellSkipIfNoPty(t)
		if *timberBin == "" {
			*timberBin = buildTestTimberBinary(parent)
		}
	}
	repository := newTestRepository(t)
	seedMiseTrustFiles(t, repository, test.miseFile, test.symlink)
	prepareMiseTrustRemote(t, repository, test.remote)
	runtime := configureMiseTrustRuntime(t, repository.runtime, test)
	runtime, logPath, upstream := installMiseTrustStub(t, repository, runtime, test)
	result := runMiseTrustCreate(t, runtime, *timberBin, upstream, test.interactive, test.answer)
	assertMiseTrustCase(t, runtime, repository, result, logPath, *timberBin, test)
}

func seedMiseTrustFiles(t *testing.T, repository testRepository, miseFile, symlink bool) {
	t.Helper()
	if !miseFile {
		return
	}
	seed := filepath.Join(resolvedTempDir(t), "seed")
	runGitCommand(t, repository.home, "clone", repository.remotePath, seed)
	runGitCommand(t, seed, "config", "user.name", "Test User")
	runGitCommand(t, seed, "config", "user.email", "test@example.com")
	if symlink {
		require.NoError(t, os.Symlink("README.md", filepath.Join(seed, "mise.toml")))
	} else {
		require.NoError(t, os.WriteFile(filepath.Join(seed, "mise.toml"), []byte("[tools]\n"), 0o600))
	}
	runGitCommand(t, seed, "add", "mise.toml")
	runGitCommand(t, seed, "commit", "-m", "add mise config")
	runGitCommand(t, seed, "push")
}

func prepareMiseTrustRemote(t *testing.T, repository testRepository, remote string) {
	t.Helper()
	// Fetch locally. The selected upstream belongs to a different remote,
	// so trusting origin by mistake would not pass the positive cases.
	runGitCommand(t, repository.barePath, "fetch", "origin")
	runGitCommand(t, repository.barePath, "remote", "add", "source", remote)
	runGitCommand(t, repository.barePath, "update-ref", "refs/remotes/source/main", "refs/remotes/origin/main")
}

func configureMiseTrustRuntime(t *testing.T, runtime timber.Runtime, test miseTrustCase) timber.Runtime {
	t.Helper()
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
	writeMiseTrustCache(t, runtime, test)
	return runtime
}

func writeMiseTrustCache(t *testing.T, runtime timber.Runtime, test miseTrustCase) {
	t.Helper()
	if test.cache == "missing" || isSame(test.cache, "refresh") {
		return
	}
	fetchedAt := time.Now().UTC()
	if test.cache == "expired" {
		fetchedAt = fetchedAt.Add(-25 * time.Hour)
	}
	organizations := `["first-org","second-org"]`
	if test.noOrganizations {
		organizations = `[]`
	}
	cache := fmt.Sprintf(`{"username":"alice","organizations":%s,"fetched_at":%q}`, organizations, fetchedAt.Format(time.RFC3339Nano))
	if test.cache == "corrupt" {
		cache = "{"
	}
	writeIdentityCache(t, runtime, cache)
}

func installMiseTrustStub(t *testing.T, repository testRepository, runtime timber.Runtime, test miseTrustCase) (timber.Runtime, string, string) {
	t.Helper()
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
	return runtime, logPath, upstream
}

func assertMiseTrustCase(t *testing.T, runtime timber.Runtime, repository testRepository, result commandResult, logPath, timberBin string, test miseTrustCase) {
	t.Helper()
	require.NoError(t, result.err)
	assert.FileExists(t, filepath.Join(repository.worktreePath("feature"), ".git"))
	if test.miseFile {
		assert.FileExists(t, filepath.Join(repository.worktreePath("feature"), "mise.toml"))
	}
	assertMiseTrustWarn(t, result, test.wantWarn)
	assertMiseTrustLog(t, repository, logPath, test.wantTrust)
	assertMiseTrustPrompt(t, runtime, result, logPath, timberBin, test)
}

func assertMiseTrustWarn(t *testing.T, result commandResult, wantWarn string) {
	t.Helper()
	if wantWarn != "" {
		assert.Contains(t, result.stderr, "warning: automatic mise trust skipped:")
		assert.Contains(t, result.stderr, wantWarn)
		return
	}
	assert.NotContains(t, result.stderr, "warning:")
}

func assertMiseTrustLog(t *testing.T, repository testRepository, logPath string, wantTrust bool) {
	t.Helper()
	if !wantTrust {
		assert.NoFileExists(t, logPath)
		return
	}
	contents, err := os.ReadFile(logPath)
	require.NoError(t, err)
	path := repository.worktreePath("feature")
	assert.Equal(t, path+"\ntrust\n"+filepath.Join(path, "mise.toml")+"\n", string(contents))
}

func assertMiseTrustPrompt(t *testing.T, runtime timber.Runtime, result commandResult, logPath, timberBin string, test miseTrustCase) {
	t.Helper()
	if !test.wantPrompt {
		assert.NotContains(t, result.stderr, "[y/N]")
		return
	}
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
	assert.True(t, settings.AutoMiseTrustDecided)
	assert.Equal(t, test.wantTrust, settings.AutoMiseTrust)
	assertMiseTrustRefusal(t, runtime, result, logPath, timberBin, test.wantTrust, test.answer)
}

func assertMiseTrustRefusal(t *testing.T, runtime timber.Runtime, result commandResult, logPath, timberBin string, wantTrust bool, answer string) {
	t.Helper()
	if !wantTrust {
		assert.Contains(t, result.stderr, "Enable it later with: timber config --auto-mise-trust")
	}
	again := interactiveCreate(t, runtime, timberBin, answer, "second")
	require.NoError(t, again.err)
	assert.NotContains(t, again.stderr, "[y/N]")
	if wantTrust {
		return
	}
	assert.NoFileExists(t, logPath)
	// The documented command can reverse a saved refusal.
	require.NoError(t, runTimberCommandWithRuntime(t, runtime, "config", "--auto-mise-trust").err)
	require.NoError(t, runTimberCommandWithRuntime(t, runtime, "create", "--upstream", "source/main", at(testRepoName, "third")).err)
	assert.FileExists(t, logPath)
}

func runMiseTrustCreate(t *testing.T, runtime timber.Runtime, timberBin, upstream string, interactive bool, answer string) commandResult {
	t.Helper()
	if interactive {
		return interactiveCreate(t, runtime, timberBin, answer, "feature")
	}
	return runTimberCommandWithRuntime(t, runtime, "create", "--upstream", upstream, at(testRepoName, "feature"))
}
