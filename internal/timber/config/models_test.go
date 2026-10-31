package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeModels(t *testing.T, contents string) Models {
	t.Helper()
	path := filepath.Join(t.TempDir(), "model.json")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	models, err := LoadModels(path)
	require.NoError(t, err)
	return models
}

const twoProviders = `{
  "$schema": "https://pi.dev/schemas/models.schema.json",
  "providers": {
    "alpha": {
      "baseUrl": "https://alpha.example/v1",
      "api": "openai-responses",
      "apiKey": "$ALPHA_KEY",
      "headers": {"X-Provider": "alpha", "X-Shared": "provider"},
      "models": [{"id": "shared-model"}, {"id": "alpha-model", "maxTokens": 2048}]
    },
    "beta": {
      "baseUrl": "https://beta.example/v1",
      "api": "openai-responses",
      "models": [{"id": "shared-model"}]
    }
  }
}`

func TestLoadModelsTreatsAnAbsentFileAsEmpty(t *testing.T) {
	models, err := LoadModels(filepath.Join(t.TempDir(), "model.json"))
	require.NoError(t, err)
	require.Equal(t, 0, models.Count())
	_, ok := models.Only()
	require.False(t, ok)
}

func TestLoadModelsRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model.json")
	require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))
	_, err := LoadModels(path)
	require.Error(t, err)
}

func TestOnlyReportsASoleModel(t *testing.T) {
	models := writeModels(t, `{"providers": {"solo": {"models": [{"id": "only-model"}]}}}`)
	selector, ok := models.Only()
	require.True(t, ok)
	require.Equal(t, "solo/only-model", selector)
	models = writeModels(t, twoProviders)
	_, ok = models.Only()
	require.False(t, ok)
}

func TestResolveModelSelector(t *testing.T) {
	models := writeModels(t, twoProviders)
	resolved, err := models.Resolve("alpha/alpha-model")
	require.NoError(t, err)
	require.Equal(t, "alpha", resolved.Provider)
	require.Equal(t, "alpha-model", resolved.Model)
	resolved, err = models.Resolve("alpha-model")
	require.NoError(t, err)
	require.Equal(t, "alpha", resolved.Provider)
	_, err = models.Resolve("alpha/missing")
	require.ErrorContains(t, err, `no model "missing" for provider "alpha"`)
	_, err = models.Resolve("missing-model")
	require.ErrorContains(t, err, `no model "missing-model"`)
	_, err = models.Resolve("shared-model")
	require.ErrorContains(t, err, "select it as provider/model")
}

func TestResolveMergesModelOverrides(t *testing.T) {
	models := writeModels(t, `{
	  "providers": {
	    "alpha": {
	      "baseUrl": "https://provider.example/v1",
	      "api": "openai-responses",
	      "apiKey": "$ALPHA_KEY",
	      "headers": {"X-Provider": "provider", "X-Shared": "provider"},
	      "models": [{
	        "id": "model",
	        "baseUrl": "https://model.example/v1",
	        "api": "openai-responses",
	        "maxTokens": 2048,
	        "headers": {"X-Shared": "model"}
	      }]
	    }
	  }
	}`)
	resolved, err := models.Resolve("model")
	require.NoError(t, err)
	require.NoError(t, resolved.Validate())
	require.Equal(t, "https://model.example/v1", resolved.BaseURL)
	require.Equal(t, 2048, resolved.MaxTokens)
	require.Equal(t, map[string]string{"X-Provider": "provider", "X-Shared": "model"}, resolved.Headers)
}

func TestValidateRejectsUnsupportedProviders(t *testing.T) {
	for _, tc := range []struct {
		name   string
		models string
		want   string
	}{
		{"other api", `{"providers": {"p": {"api": "openai-completions", "models": [{"id": "m"}]}}}`, `requires "openai-responses"`},
		{"missing api", `{"providers": {"p": {"baseUrl": "https://example/v1", "models": [{"id": "m"}]}}}`, `uses api ""`},
		{"oauth", `{"providers": {"p": {"api": "openai-responses", "oauth": "radius", "models": [{"id": "m"}]}}}`, "OAuth flow"},
		{"negative budget", `{"providers": {"p": {"api": "openai-responses", "models": [{"id": "m", "maxTokens": -1}]}}}`, "negative maxTokens"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			models := writeModels(t, tc.models)
			resolved, err := models.Resolve("m")
			require.NoError(t, err)
			require.ErrorContains(t, resolved.Validate(), tc.want)
		})
	}
}

func TestValueExpandsLikePi(t *testing.T) {
	t.Setenv("MODELS_TEST_KEY", "secret")
	for _, tc := range []struct {
		name  string
		value string
		want  string
	}{
		{"literal", "sk-literal", "sk-literal"},
		{"variable", "$MODELS_TEST_KEY", "secret"},
		{"braced variable", "${MODELS_TEST_KEY}", "secret"},
		{"escaped dollar", "$$MODELS_TEST_KEY", "$MODELS_TEST_KEY"},
		{"escaped exclamation", "$!literal", "!literal"},
		{"trailing dollar", "key$", "key$"},
		{"embedded variable", "prefix-$MODELS_TEST_KEY-suffix", "prefix-secret-suffix"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, err := Value(tc.value)
			require.NoError(t, err)
			require.Equal(t, tc.want, value)
		})
	}
}

// writeSecurityStub installs an executable stub as the security binary and
// restores the real path when the test ends. The stub prints each argument
// as <arg> so tests can see exactly how values split.
func writeSecurityStub(t *testing.T, script string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "security")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o700))
	previous := securityBinary
	securityBinary = path
	t.Cleanup(func() { securityBinary = previous })
}

func TestValueRunsOnlySecurity(t *testing.T) {
	writeSecurityStub(t, "#!/bin/sh\nprintf '<%s>' \"$@\"")
	for _, tc := range []struct {
		name  string
		value string
		want  string
	}{
		{"simple arguments", "!security find-generic-password -w", "<find-generic-password><-w>"},
		{"quoted arguments", "!security -s 'my service' -a \"my account\"", "<-s><my service><-a><my account>"},
		{"escaped space", `!security -s my\ service`, "<-s><my service>"},
		{"shell stays literal", "!security '$(touch /tmp/timber-pwned)'", "<$(touch /tmp/timber-pwned)>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, err := Value(tc.value)
			require.NoError(t, err)
			require.Equal(t, tc.want, value)
		})
	}
}

func TestValueRejectsUnusableSources(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  string
	}{
		{"unset variable", "$MODELS_TEST_UNSET", "MODELS_TEST_UNSET is not set"},
		{"unterminated variable", "${MODELS_TEST_KEY", "unterminated variable"},
		{"other command", "!printf key-from-command", "only \"!security ...\""},
		{"bare exclamation", "!", "only \"!security ...\""},
		{"security prefix", "!securityx find-generic-password", "only \"!security ...\""},
		{"missing arguments", "!security", "requires arguments"},
		{"unterminated quote", "!security -s 'my service", "unterminated quote"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MODELS_TEST_UNSET", "")
			_, err := Value(tc.value)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestValueReportsSecurityFailures(t *testing.T) {
	writeSecurityStub(t, "#!/bin/sh\necho keychain-error >&2\nexit 1")
	_, err := Value("!security find-generic-password -s missing -w")
	require.ErrorContains(t, err, "exit status 1")
	require.ErrorContains(t, err, "keychain-error")
	writeSecurityStub(t, "#!/bin/sh\ntrue")
	_, err = Value("!security find-generic-password -s missing -w")
	require.ErrorContains(t, err, "returned no value")
}

func TestValuesExpandsHeaders(t *testing.T) {
	t.Setenv("MODELS_TEST_KEY", "secret")
	values, err := Values(map[string]string{"Authorization": "Bearer $MODELS_TEST_KEY"})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"Authorization": "Bearer secret"}, values)
	_, err = Values(map[string]string{"X-Key": "$MODELS_TEST_UNSET"})
	require.ErrorContains(t, err, "header X-Key")
}
