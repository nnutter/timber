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

func configureHKInstallRuntime(t *testing.T, runtime timber.Runtime, enabled, explicitDisable, interactive bool, cache string, noOrganizations bool) timber.Runtime {
	t.Helper()
	runtime.GhExecutable = filepath.Join(resolvedTempDir(t), "missing-gh")
	if enabled {
		result := runTimberCommandWithRuntime(t, runtime, "config", "--auto-hk-install")
		require.NoError(t, result.err)
	}
	if explicitDisable {
		result := runTimberCommandWithRuntime(t, runtime, "config", "--no-auto-hk-install")
		require.NoError(t, result.err)
	}
	if interactive {
		// Keep automatic mise trust from asking its own question first.
		result := runTimberCommandWithRuntime(t, runtime, "config", "--no-auto-mise-trust")
		require.NoError(t, result.err)
	}
	writeHKInstallCache(t, runtime, cache, noOrganizations)
	return runtime
}

func writeHKInstallCache(t *testing.T, runtime timber.Runtime, cache string, noOrganizations bool) {
	t.Helper()
	if cache == "missing" {
		return
	}
	fetchedAt := time.Now().UTC()
	if cache == "expired" {
		fetchedAt = fetchedAt.Add(-25 * time.Hour)
	}
	organizations := `["first-org","second-org"]`
	if noOrganizations {
		organizations = `[]`
	}
	encoded := fmt.Sprintf(`{"username":"alice","organizations":%s,"fetched_at":%q}`, organizations, fetchedAt.Format(time.RFC3339Nano))
	writeIdentityCache(t, runtime, encoded)
}

func assertHKInstallCase(t *testing.T, runtime timber.Runtime, repository testRepository, result commandResult, logPath, timberBin string, test struct {
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
},
) {
	t.Helper()
	assert.FileExists(t, filepath.Join(repository.worktreePath("feature"), ".git"))
	assertHKInstallWarn(t, result, test.wantWarn)
	assertHKInstallLog(t, repository, logPath, test.wantInstall, test.wantMise)
	assertHKInstallPrompt(t, runtime, result, logPath, timberBin, test.wantPrompt, test.wantInstall, test.noOrganizations, test.answer)
}

func assertHKInstallWarn(t *testing.T, result commandResult, wantWarn string) {
	t.Helper()
	if wantWarn != "" {
		assert.Contains(t, result.stderr, "warning: automatic hk install skipped:")
		assert.Contains(t, result.stderr, wantWarn)
		return
	}
	assert.NotContains(t, result.stderr, "warning:")
}

func assertHKInstallLog(t *testing.T, repository testRepository, logPath string, wantInstall, wantMise bool) {
	t.Helper()
	if !wantInstall {
		assert.NoFileExists(t, logPath)
		return
	}
	contents, err := os.ReadFile(logPath)
	require.NoError(t, err)
	want := repository.worktreePath("feature") + "\ninstall\n"
	if wantMise {
		want += "--mise\n"
	}
	assert.Equal(t, want, string(contents))
}

func assertHKInstallPrompt(t *testing.T, runtime timber.Runtime, result commandResult, logPath, timberBin string, wantPrompt, wantInstall, noOrganizations bool, answer string) {
	t.Helper()
	if !wantPrompt {
		assert.NotContains(t, result.stderr, "[y/N]")
		return
	}
	assert.Contains(t, result.stderr, "permits code execution")
	assert.Contains(t, result.stderr, "[y/N]")
	assert.Contains(t, result.stderr, "GitHub username: alice")
	organizations := "GitHub organizations: first-org, second-org"
	if noOrganizations {
		organizations = "GitHub organizations: none"
	}
	assert.Contains(t, result.stderr, organizations)
	assert.Less(t, strings.Index(result.stderr, organizations), strings.Index(result.stderr, "[y/N]"))
	settings, err := runtime.LoadConfig()
	require.NoError(t, err)
	assert.True(t, settings.AutoHKInstallDecided)
	assert.Equal(t, wantInstall, settings.AutoHKInstall)
	assertHKInstallRefusal(t, runtime, result, logPath, timberBin, wantInstall, answer)
}

func assertHKInstallRefusal(t *testing.T, runtime timber.Runtime, result commandResult, logPath, timberBin string, wantInstall bool, answer string) {
	t.Helper()
	if !wantInstall {
		assert.Contains(t, result.stderr, "Enable it later with: timber config --auto-hk-install")
	}
	again := interactiveCreate(t, runtime, timberBin, answer, "second")
	require.NoError(t, again.err)
	assert.NotContains(t, again.stderr, "[y/N]")
	if wantInstall {
		return
	}
	assert.NoFileExists(t, logPath)
	// The documented command can reverse a saved refusal.
	require.NoError(t, runTimberCommandWithRuntime(t, runtime, "config", "--auto-hk-install").err)
	require.NoError(t, runTimberCommandWithRuntime(t, runtime, "create", "--upstream", "source/main", at(testRepoName, "third")).err)
	assert.FileExists(t, logPath)
}
