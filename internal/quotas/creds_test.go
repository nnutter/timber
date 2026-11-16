package quotas

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXAITokenUsesClientSignInsInOrder(t *testing.T) {
	t.Parallel()
	const grok = `{"https://auth.x.ai::client-id":{"auth_mode":"oidc","key":"grok-token"}}`
	const pi = `{"xai":{"type":"oauth","access":"pi-token","refresh":"pi-refresh"}}`
	const hermes = `{"providers":{"xai-oauth":{"tokens":{"access_token":"hermes-token"}}}}`
	cases := []struct {
		name   string
		grok   string
		pi     string
		hermes string
		want   string
	}{
		{name: "Grok before Pi and Hermes", grok: grok, pi: pi, hermes: hermes, want: "grok-token"},
		{name: "Pi before Hermes", pi: pi, hermes: hermes, want: "pi-token"},
		{name: "Hermes provider", hermes: hermes, want: "hermes-token"},
		{name: "Hermes pool", hermes: `{"credential_pool":{"xai-oauth":[{"access_token":"pool-token"}]}}`, want: "pool-token"},
		{name: "malformed Grok", grok: `{`, pi: pi, want: "pi-token"},
		{name: "malformed Pi", pi: `[]`, hermes: hermes, want: "hermes-token"},
		{name: "empty Grok token", grok: `{"https://auth.x.ai::client-id":{"auth_mode":"oidc","key":""}}`, pi: pi, want: "pi-token"},
		{name: "Pi refresh alone", pi: `{"xai":{"type":"oauth","refresh":"not-an-access-token"}}`, hermes: hermes, want: "hermes-token"},
		{name: "Grok API key", grok: `{"https://auth.x.ai::client-id":{"auth_mode":"api_key","key":"not-a-plan-token"}}`, pi: pi, want: "pi-token"},
		{name: "unrelated issuer", grok: `{"https://other.example::client-id":{"auth_mode":"oidc","key":"not-a-grok-token"}}`, pi: pi, want: "pi-token"},
		{name: "Pi API key", pi: `{"xai":{"type":"api_key","key":"not-a-plan-token","access":"not-oauth"}}`},
		{name: "no sign-in"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			files := map[string]string{
				filepath.Join(home, ".grok", "auth.json"):        tc.grok,
				filepath.Join(home, ".pi", "agent", "auth.json"): tc.pi,
				filepath.Join(home, ".hermes", "auth.json"):      tc.hermes,
			}
			for path, body := range files {
				if body == "" {
					continue
				}
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
				require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
			}
			source := Source{UserHome: func() string { return home }, Env: func(string) string { return "" }}
			token, err := source.XAIToken()
			if tc.want == "" {
				var missing MissingError
				require.ErrorAs(t, err, &missing)
				assert.Contains(t, err.Error(), "Sign in to Grok")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.want, token)
		})
	}
}

func TestDotEnvBeatsAStaleEnvironmentKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HERMES_HOME", home)
	t.Setenv("DEEPSEEK_API_KEY", "from-env")
	require.NoError(t, os.WriteFile(filepath.Join(home, ".env"), []byte("DEEPSEEK_API_KEY=from-dotenv\n"), 0o600))

	key, err := (Source{}).APIKey("deepseek", []string{"DEEPSEEK_API_KEY"}, "DeepSeek")
	require.NoError(t, err)
	assert.Equal(t, "from-dotenv", key)
}

func TestPoolSuppliesAKeyTheEnvironmentDoesNot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HERMES_HOME", home)
	t.Setenv("ZAI_API_KEY", "")
	t.Setenv("GLM_API_KEY", "")
	t.Setenv("Z_AI_API_KEY", "")
	require.NoError(t, os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"credential_pool":{"zai":[{"access_token":"pool-zai"}]}}`), 0o600))

	key, err := (Source{}).APIKey("zai", []string{"GLM_API_KEY", "ZAI_API_KEY", "Z_AI_API_KEY"}, "Z.ai")
	require.NoError(t, err)
	assert.Equal(t, "pool-zai", key)
}

func TestConsoleAPIKeyIsNotAClaudePlan(t *testing.T) {
	t.Setenv("HERMES_HOME", t.TempDir())
	t.Setenv("ANTHROPIC_TOKEN", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-api-test")

	_, err := (Source{}).AnthropicPlanToken()
	var missing MissingError
	require.ErrorAs(t, err, &missing)
	assert.Contains(t, missing.Error(), "Console API key")
}

func TestClaudeCodeFileSuppliesAPlanToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HERMES_HOME", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("ANTHROPIC_TOKEN", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".claude", ".credentials.json"), []byte(`{"claudeAiOauth":{"accessToken":"sk-ant-oat-file"}}`), 0o600))

	token, err := (Source{UserHome: func() string { return home }}).AnthropicPlanToken()
	require.NoError(t, err)
	assert.Equal(t, "sk-ant-oat-file", token)
}
