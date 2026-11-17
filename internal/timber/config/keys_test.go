package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigKeysExposeSettingsInNameOrder(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"auto_hk_install", "auto_mise_trust", "strip_github_owners"}, KeyNames())
	for _, key := range Keys() {
		assert.NotEmpty(t, key.Description, "setting %q needs a description", key.Name)
	}
	// The keys that only remember a setup-prompt answer are not configurable.
	_, err := Get(Settings{}, "auto_mise_trust_decided")
	require.Error(t, err)
}

func TestConfigGet(t *testing.T) {
	t.Parallel()
	settings := Settings{AutoMiseTrust: true, StripGitHubOwners: []string{"nnutter"}}

	values, err := Get(settings, "auto_mise_trust")
	require.NoError(t, err)
	assert.Equal(t, []string{"true"}, values)

	values, err = Get(settings, "strip_github_owners")
	require.NoError(t, err)
	assert.Equal(t, []string{"nnutter"}, values)

	values, err = Get(Settings{}, "strip_github_owners")
	require.NoError(t, err)
	assert.Empty(t, values)

	_, err = Get(settings, "future_setting")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auto_hk_install, auto_mise_trust, strip_github_owners")
}

func TestConfigSetBoolRecordsTheDecision(t *testing.T) {
	t.Parallel()
	var settings Settings
	require.NoError(t, Set(&settings, "auto_mise_trust", []string{"true"}))
	assert.True(t, settings.AutoMiseTrust)
	assert.True(t, settings.AutoMiseTrustDecided)

	require.NoError(t, Set(&settings, "auto_hk_install", []string{"false"}))
	assert.False(t, settings.AutoHKInstall)
	assert.True(t, settings.AutoHKInstallDecided)
}

func TestConfigSetRejectsInvalidValues(t *testing.T) {
	t.Parallel()
	settings := Settings{StripGitHubOwners: []string{"nnutter"}}
	require.Error(t, Set(&settings, "auto_mise_trust", []string{"yes"}))
	require.Error(t, Set(&settings, "auto_mise_trust", []string{"true", "false"}))
	require.Error(t, Set(&settings, "strip_github_owners", []string{"nnutter/timber"}))
	// A rejected value leaves the setting untouched.
	assert.Equal(t, []string{"nnutter"}, settings.StripGitHubOwners)
}

func TestConfigSetListReplacesValues(t *testing.T) {
	t.Parallel()
	settings := Settings{StripGitHubOwners: []string{"nnutter"}}
	require.NoError(t, Set(&settings, "strip_github_owners", []string{"acme"}))
	assert.Equal(t, []string{"acme"}, settings.StripGitHubOwners)
}

func TestConfigAppend(t *testing.T) {
	t.Parallel()
	var settings Settings
	require.NoError(t, Append(&settings, "strip_github_owners", []string{"nnutter", "acme"}))
	require.NoError(t, Append(&settings, "strip_github_owners", []string{"NNutter"}))
	assert.Equal(t, []string{"acme", "nnutter"}, settings.StripGitHubOwners)
	require.Error(t, Append(&settings, "auto_mise_trust", []string{"true"}))
}

func TestConfigRemove(t *testing.T) {
	t.Parallel()
	settings := Settings{
		AutoMiseTrust:        true,
		AutoMiseTrustDecided: true,
		StripGitHubOwners:    []string{"acme", "nnutter"},
	}

	require.NoError(t, Remove(&settings, "strip_github_owners", []string{"ACME"}))
	assert.Equal(t, []string{"nnutter"}, settings.StripGitHubOwners)
	require.Error(t, Remove(&settings, "auto_mise_trust", []string{"true"}))

	require.NoError(t, Remove(&settings, "auto_mise_trust", nil))
	assert.False(t, settings.AutoMiseTrust)
	assert.False(t, settings.AutoMiseTrustDecided)

	require.NoError(t, Remove(&settings, "strip_github_owners", nil))
	assert.Empty(t, settings.StripGitHubOwners)
}
