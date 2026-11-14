package quotas

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
