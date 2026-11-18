package quotas

import (
	"context"
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
			token, _, err := source.XAIToken()
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

func TestPiAPIKeysPrecedeHermesButNotEnvironment(t *testing.T) {
	t.Parallel()
	providers := []struct {
		id     string
		envKey string
	}{
		{id: "ollama-cloud", envKey: "OLLAMA_API_KEY"},
		{id: "opencode-go", envKey: "OPENCODE_GO_API_KEY"},
	}
	cases := []struct {
		name  string
		entry string
		env   string
		want  string
	}{
		{name: "Pi before Hermes dotenv and pool", entry: `{"type":"api_key","key":"pi-key"}`, want: "pi-key"},
		{name: "environment override", entry: `{"type":"api_key","key":"pi-key"}`, env: "env-key", want: "env-key"},
		{name: "missing Pi entry", entry: `null`, want: "hermes-key"},
		{name: "empty Pi key", entry: `{"type":"api_key","key":""}`, want: "hermes-key"},
		{name: "OAuth is not an API key", entry: `{"type":"oauth","key":"wrong-type","access":"oauth-token"}`, want: "hermes-key"},
	}
	for _, provider := range providers {
		for _, tc := range cases {
			t.Run(provider.id+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				source := envSource(map[string]string{provider.envKey: tc.env})
				source.ReadFile = func(path string) ([]byte, error) {
					switch path {
					case filepath.Join(".pi", "agent", "auth.json"):
						return []byte(`{"` + provider.id + `":` + tc.entry + `}`), nil
					case ".env":
						return []byte(provider.envKey + "=hermes-key"), nil
					case "auth.json":
						return []byte(`{"credential_pool":{"` + provider.id + `":[{"access_token":"pool-key"}]}}`), nil
					}
					return nil, os.ErrNotExist
				}
				key, err := source.APIKey(provider.id, []string{provider.envKey}, provider.id)
				require.NoError(t, err)
				assert.Equal(t, tc.want, key)
			})
		}
	}
}

func TestCopilotUsesPiBeforeGitHubCLI(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		entry string
		env   string
		want  string
	}{
		{name: "Pi OAuth", entry: `{"type":"oauth","access":"pi-token","refresh":"pi-refresh"}`, want: "pi-token"},
		{name: "environment override", entry: `{"type":"oauth","access":"pi-token"}`, env: "env-token", want: "env-token"},
		{name: "GitHub CLI fallback", entry: `null`, want: "gh-token"},
		{name: "refresh is not access", entry: `{"type":"oauth","refresh":"pi-refresh"}`, want: "gh-token"},
		{name: "API key is not OAuth", entry: `{"type":"api_key","access":"wrong-type","key":"pi-key"}`, want: "gh-token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := envSource(map[string]string{"GH_TOKEN": tc.env})
			source.ReadFile = func(path string) ([]byte, error) {
				if path == filepath.Join(".pi", "agent", "auth.json") {
					return []byte(`{"github-copilot":` + tc.entry + `}`), nil
				}
				return nil, os.ErrNotExist
			}
			source.LookPath = func(string) (string, error) { return "gh", nil }
			source.Run = func(context.Context, string, ...string) ([]byte, error) {
				assert.Equal(t, "gh-token", tc.want, "GitHub CLI should only run as a fallback")
				return []byte("gh-token\n"), nil
			}
			token, err := source.CopilotToken(t.Context())
			require.NoError(t, err)
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
