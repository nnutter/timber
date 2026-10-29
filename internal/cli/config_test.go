package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigPersistsSettingsUnderXDGConfigHome(t *testing.T) {
	runtime := testRuntime(t)
	runtime.ConfigHome = filepath.Join(resolvedTempDir(t), "custom-config")
	for _, step := range []struct {
		args []string
		want string
	}{
		{[]string{"config"}, "auto_mise_trust = false\nauto_hk_install = false\n"},
		{[]string{"config", "--auto-mise-trust"}, "auto_mise_trust = true\nauto_mise_trust_decided = true\nauto_hk_install = false\n"},
		{[]string{"config"}, "auto_mise_trust = true\nauto_mise_trust_decided = true\nauto_hk_install = false\n"},
		{[]string{"config", "--no-auto-mise-trust"}, "auto_mise_trust = false\nauto_mise_trust_decided = true\nauto_hk_install = false\n"},
		{[]string{"config"}, "auto_mise_trust = false\nauto_mise_trust_decided = true\nauto_hk_install = false\n"},
		{[]string{"config", "--auto-hk-install"}, "auto_mise_trust = false\nauto_mise_trust_decided = true\nauto_hk_install = true\nauto_hk_install_decided = true\n"},
		{[]string{"config"}, "auto_mise_trust = false\nauto_mise_trust_decided = true\nauto_hk_install = true\nauto_hk_install_decided = true\n"},
		{[]string{"config", "--no-auto-hk-install"}, "auto_mise_trust = false\nauto_mise_trust_decided = true\nauto_hk_install = false\nauto_hk_install_decided = true\n"},
		{[]string{"config"}, "auto_mise_trust = false\nauto_mise_trust_decided = true\nauto_hk_install = false\nauto_hk_install_decided = true\n"},
	} {
		result := runTimberCommandWithRuntime(t, runtime, step.args...)
		require.NoError(t, result.err)
		assert.Equal(t, step.want, result.stdout)
		contents, err := os.ReadFile(filepath.Join(runtime.ConfigHome, "timber", "config.toml"))
		require.NoError(t, err)
		assert.Equal(t, step.want, string(contents))
	}
	result := runTimberCommandWithRuntime(t, runtime, "config", "--auto-mise-trust", "--no-auto-mise-trust")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "none of the others can be")
	result = runTimberCommandWithRuntime(t, runtime, "config", "--auto-hk-install", "--no-auto-hk-install")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "none of the others can be")
}

func TestConfigDoesNotOverwriteInvalidOrUnknownSettings(t *testing.T) {
	for _, contents := range []string{"auto_mise_trust = \"yes\"\n", "future_setting = true\n"} {
		t.Run(contents, func(t *testing.T) {
			runtime := testRuntime(t)
			path := filepath.Join(runtime.ConfigHome, "timber", "config.toml")
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
			result := runTimberCommandWithRuntime(t, runtime, "config", "--auto-mise-trust")
			require.Error(t, result.err)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, contents, string(after))
		})
	}
}
