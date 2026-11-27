package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigShowUsesXDGConfigHome(t *testing.T) {
	runtime := testRuntime(t)
	runtime.ConfigHome = filepath.Join(resolvedTempDir(t), "custom-config")
	configPath := filepath.Join(runtime.ConfigHome, "timber", "config.toml")
	_, statErr := os.Stat(configPath)
	require.ErrorIs(t, statErr, os.ErrNotExist)

	result := runTimberCommandWithRuntime(t, runtime, "config", "show")
	require.NoError(t, result.err)
	want := "auto_mise_trust = false\nauto_hk_install = false\n"
	assert.Equal(t, want, result.stdout)
	contents, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, want, string(contents))
}

func TestConfigSetAndGet(t *testing.T) {
	runtime := testRuntime(t)
	set := runTimberCommandWithRuntime(t, runtime, "config", "set", "auto_mise_trust", "true")
	require.NoError(t, set.err)
	assert.Empty(t, set.stdout)

	show := runTimberCommandWithRuntime(t, runtime, "config", "show")
	require.NoError(t, show.err)
	assert.Equal(t, "auto_mise_trust = true\nauto_mise_trust_decided = true\nauto_hk_install = false\n", show.stdout)

	get := runTimberCommandWithRuntime(t, runtime, "config", "get", "auto_mise_trust")
	require.NoError(t, get.err)
	assert.Equal(t, "true\n", get.stdout)

	get = runTimberCommandWithRuntime(t, runtime, "config", "get", "strip_github_owners")
	require.NoError(t, get.err)
	assert.Empty(t, get.stdout)

	require.NoError(t, runTimberCommandWithRuntime(t, runtime, "config", "set", "strip_github_owners", "nnutter").err)
	get = runTimberCommandWithRuntime(t, runtime, "config", "get", "strip_github_owners")
	require.NoError(t, get.err)
	assert.Equal(t, "nnutter\n", get.stdout)
}

func TestConfigAliasCanBeInvokedAndRemoved(t *testing.T) {
	t.Parallel()
	runtime := testRuntime(t)
	result := runTimberCommandWithRuntime(t, runtime, "config", "alias.settings", "config get")
	require.NoError(t, result.err, result.stderr)

	result = runTimberCommandWithRuntime(t, runtime, "settings", "auto_mise_trust")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, "false\n", result.stdout)

	result = runTimberCommandWithRuntime(t, runtime, "config", "get", "alias.settings")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, "config get\n", result.stdout)

	result = runTimberCommandWithRuntime(t, runtime, "config", "remove", "alias.settings")
	require.NoError(t, result.err, result.stderr)
	result = runTimberCommandWithRuntime(t, runtime, "settings", "auto_mise_trust")
	require.ErrorContains(t, result.err, "settings")
}

func TestConfigAppendAndRemoveListValues(t *testing.T) {
	runtime := testRuntime(t)
	require.NoError(t, runTimberCommandWithRuntime(t, runtime, "config", "append", "strip_github_owners", "nnutter", "acme").err)
	require.NoError(t, runTimberCommandWithRuntime(t, runtime, "config", "append", "strip_github_owners", "NNutter").err)

	get := runTimberCommandWithRuntime(t, runtime, "config", "get", "strip_github_owners")
	require.NoError(t, get.err)
	assert.Equal(t, "acme\nnnutter\n", get.stdout)

	require.NoError(t, runTimberCommandWithRuntime(t, runtime, "config", "rm", "strip_github_owners", "ACME").err)
	get = runTimberCommandWithRuntime(t, runtime, "config", "get", "strip_github_owners")
	require.NoError(t, get.err)
	assert.Equal(t, "nnutter\n", get.stdout)

	require.NoError(t, runTimberCommandWithRuntime(t, runtime, "config", "remove", "strip_github_owners").err)
	get = runTimberCommandWithRuntime(t, runtime, "config", "get", "strip_github_owners")
	require.NoError(t, get.err)
	assert.Empty(t, get.stdout)
}

func TestConfigRemoveBoolClearsTheDecision(t *testing.T) {
	runtime := testRuntime(t)
	require.NoError(t, runTimberCommandWithRuntime(t, runtime, "config", "set", "auto_hk_install", "true").err)
	require.NoError(t, runTimberCommandWithRuntime(t, runtime, "config", "remove", "auto_hk_install").err)

	show := runTimberCommandWithRuntime(t, runtime, "config", "show")
	require.NoError(t, show.err)
	assert.Equal(t, "auto_mise_trust = false\nauto_hk_install = false\n", show.stdout)
}

func TestConfigRejectsUnknownSettingsAndInvalidValues(t *testing.T) {
	runtime := testRuntime(t)
	for _, args := range [][]string{
		{"config", "get", "future_setting"},
		{"config", "set", "future_setting", "true"},
		{"config", "set", "auto_mise_trust", "yes"},
		{"config", "set", "strip_github_owners", "nnutter/timber"},
		{"config", "append", "auto_mise_trust", "true"},
		{"config", "remove", "auto_mise_trust", "true"},
	} {
		result := runTimberCommandWithRuntime(t, runtime, args...)
		require.Error(t, result.err, "args=%v", args)
	}

	// None of the failures changed a setting.
	show := runTimberCommandWithRuntime(t, runtime, "config", "show")
	require.NoError(t, show.err)
	assert.Equal(t, "auto_mise_trust = false\nauto_hk_install = false\n", show.stdout)
}

func TestConfigWithoutSubcommandShowsHelp(t *testing.T) {
	runtime := testRuntime(t)
	result := runTimberCommandWithRuntime(t, runtime, "config")
	require.NoError(t, result.err)
	assert.Contains(t, result.stdout, "show")
	assert.Contains(t, result.stdout, "append")

	// Help must not create the settings file.
	_, err := os.Stat(filepath.Join(runtime.ConfigHome, "timber", "config.toml"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestConfigDoesNotOverwriteInvalidOrUnknownSettings(t *testing.T) {
	for _, contents := range []string{"auto_mise_trust = \"yes\"\n", "future_setting = true\n"} {
		t.Run(contents, func(t *testing.T) {
			runtime := testRuntime(t)
			path := filepath.Join(runtime.ConfigHome, "timber", "config.toml")
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
			result := runTimberCommandWithRuntime(t, runtime, "config", "set", "auto_mise_trust", "true")
			require.Error(t, result.err)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, contents, string(after))
		})
	}
}
