package quotas

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	json "encoding/json/v2"
)

const commandWait = 10 * time.Second

// Source reads the sign-ins clients already store on this machine.
// A zero Source reads the process environment, HERMES_HOME, and the user home.
type Source struct {
	Env      func(string) string
	Hermes   func() string
	UserHome func() string
	LookPath func(string) (string, error)
	Run      func(context.Context, string, ...string) ([]byte, error)
	ReadFile func(string) ([]byte, error)
}

// APIKey reads provider keys, including Pi keys for Ollama Cloud and OpenCode Go.
func (s Source) APIKey(provider string, vars []string, what string) (string, string, error) {
	switch provider {
	case "ollama-cloud", "opencode-go":
		for _, key := range vars {
			if value := strings.TrimSpace(s.env(key)); value != "" {
				return value, key + " environment variable", nil
			}
		}
		if token := s.piToken(provider, "api_key", "key"); token != "" {
			return token, "Pi", nil
		}
	}
	dotenv := s.dotEnv()
	for _, key := range vars {
		if value, location := preferDotEnv(dotenv, key, s.env(key)); value != "" {
			return value, location, nil
		}
	}
	if token := s.poolToken(provider); token != "" {
		return token, "Hermes", nil
	}
	return "", "", MissingError{Text: fmt.Sprintf("Add a %s key in Hermes to read its plan.", what)}
}

// AnthropicPlanToken is Claude's plan sign-in. A Console API key has no plan.
func (s Source) AnthropicPlanToken() (string, string, error) {
	dotenv := s.dotEnv()
	if token, location := firstSecret(dotenv, s.env, "ANTHROPIC_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN"); token != "" {
		return requirePlanToken(token, location)
	}
	if token, location := preferDotEnv(dotenv, "ANTHROPIC_API_KEY", s.env("ANTHROPIC_API_KEY")); token != "" {
		return requirePlanToken(token, location)
	}
	if token := s.poolToken("anthropic"); claudeOAuth(token) {
		return token, "Hermes", nil
	}
	if token := s.claudeCodeToken(); claudeOAuth(token) {
		return token, "Claude Code", nil
	}
	if token := s.hermesOAuthToken(); claudeOAuth(token) {
		return token, "Hermes", nil
	}
	return "", "", MissingError{Text: "Sign in to Claude in Hermes to read its plan."}
}

// Command finds a service CLI from an override, PATH, or mise's shims.
func (s Source) Command(overrideEnv, name, missing string) (string, error) {
	if override := strings.TrimSpace(s.env(overrideEnv)); override != "" {
		return override, nil
	}
	if path, err := s.look(name); err == nil && nonempty(path) {
		return path, nil
	}
	if shim := s.shim(name); shim != "" {
		return shim, nil
	}
	return "", MissingError{Text: missing}
}

// CopilotToken is the GitHub token behind Copilot's premium requests.
func (s Source) CopilotToken(ctx context.Context) (string, string, error) {
	for _, key := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if value := strings.TrimSpace(s.env(key)); value != "" {
			return value, key + " environment variable", nil
		}
	}
	if token := s.piToken("github-copilot", "oauth", "access"); token != "" {
		return token, "Pi", nil
	}
	gh, err := s.look("gh")
	if err != nil || blank(gh) {
		gh = s.shim("gh")
	}
	if gh == "" {
		return "", "", MissingError{Text: "Sign in with the GitHub CLI (`gh auth login`) to read Copilot's plan."}
	}
	output, err := s.run(ctx, gh, "auth", "token")
	token := strings.TrimSpace(string(output))
	if err != nil || blank(token) {
		return "", "", MissingError{Text: "Sign in with the GitHub CLI (`gh auth login`) to read Copilot's plan."}
	}
	return token, "GitHub CLI", nil
}

// MiniMaxToken is a Coding Plan key, or the OAuth access token for that plan.
func (s Source) MiniMaxToken() (string, string, error) {
	token, location, err := s.APIKey("minimax", []string{"MINIMAX_API_KEY"}, "MiniMax")
	if err == nil {
		return token, location, nil
	}
	if oauth := s.providerToken("minimax-oauth"); oauth != "" {
		return oauth, "Hermes", nil
	}
	if oauth := s.poolToken("minimax-oauth"); oauth != "" {
		return oauth, "Hermes", nil
	}
	return "", "", err
}

// XAIToken is a SuperGrok or X Premium sign-in, never an xAI API key.
func (s Source) XAIToken() (string, string, error) {
	if token := s.grokCLIToken(); token != "" {
		return token, "Grok CLI", nil
	}
	if token := s.piToken("xai", "oauth", "access"); token != "" {
		return token, "Pi", nil
	}
	if token := s.providerToken("xai-oauth"); token != "" {
		return token, "Hermes", nil
	}
	if token := s.poolToken("xai-oauth"); token != "" {
		return token, "Hermes", nil
	}
	return "", "", MissingError{Text: "Sign in to Grok with the Grok CLI, Pi, or Hermes to read its plan."}
}

func (s Source) authDocuments() []map[string]any {
	homes := []string{s.hermesHome()}
	if filepath.Base(filepath.Dir(homes[0])) == "profiles" {
		homes = append(homes, filepath.Dir(filepath.Dir(homes[0])))
	}
	documents := make([]map[string]any, 0, len(homes))
	for _, home := range homes {
		body, err := s.read(filepath.Join(home, "auth.json"))
		if err != nil {
			continue
		}
		var decoded any
		if err := json.Unmarshal(body, &decoded); err != nil {
			continue
		}
		document, ok := asObject(decoded)
		if !ok {
			continue
		}
		documents = append(documents, document)
	}
	return documents
}

func (s Source) claudeCodeToken() string {
	dir := strings.TrimSpace(s.env("CLAUDE_CONFIG_DIR"))
	if dir == "" {
		dir = filepath.Join(s.userHome(), ".claude")
	}
	return oauthAccessToken(s.readJSON(filepath.Join(dir, ".credentials.json")), "claudeAiOauth", "accessToken")
}

func (s Source) dotEnv() map[string]string {
	body, err := s.read(filepath.Join(s.hermesHome(), ".env"))
	if err != nil {
		return nil
	}
	return parseDotEnv(string(body))
}

func (s Source) env(key string) string {
	if s.Env != nil {
		return s.Env(key)
	}
	return os.Getenv(key)
}

func (s Source) grokCLIToken() string {
	document := s.readJSON(filepath.Join(s.userHome(), ".grok", "auth.json"))
	for id, value := range document {
		if !strings.HasPrefix(id, "https://auth.x.ai::") {
			continue
		}
		state, ok := asObject(value)
		if !ok {
			continue
		}
		if state["auth_mode"] != "oidc" {
			continue
		}
		if token := oauthAccessToken(state, "", "key"); token != "" {
			return token
		}
	}
	return ""
}

func (s Source) hermesHome() string {
	if s.Hermes != nil {
		return s.Hermes()
	}
	if home := strings.TrimSpace(s.env("HERMES_HOME")); home != "" {
		return expandHome(home, s.userHome())
	}
	return filepath.Join(s.userHome(), ".hermes"+s.env("HERMES_DATA_DIR_SUFFIX"))
}

func (s Source) hermesOAuthToken() string {
	return oauthAccessToken(s.readJSON(filepath.Join(s.hermesHome(), ".anthropic_oauth.json")), "", "accessToken")
}

func (s Source) look(name string) (string, error) {
	if s.LookPath != nil {
		return s.LookPath(name)
	}
	return exec.LookPath(name)
}

func (s Source) piToken(provider, kind, field string) string {
	document := s.readJSON(filepath.Join(s.userHome(), ".pi", "agent", "auth.json"))
	state, ok := asObject(document[provider])
	if !ok {
		return ""
	}
	if state["type"] != kind {
		return ""
	}
	return oauthAccessToken(state, "", field)
}

func (s Source) poolToken(provider string) string {
	for _, document := range s.authDocuments() {
		entries, present := poolEntries(document, provider)
		if present {
			return firstToken(entries)
		}
	}
	return ""
}

func (s Source) providerToken(ids ...string) string {
	for index, document := range s.authDocuments() {
		token := accessToken(document, ids...)
		if token != "" {
			return token
		}
		// A profile store that has the provider with no token shadows the root store.
		if index == 0 && providerPresent(document, ids...) {
			return ""
		}
	}
	return ""
}

func (s Source) read(path string) ([]byte, error) {
	if s.ReadFile != nil {
		return s.ReadFile(path)
	}
	return os.ReadFile(path)
}

func (s Source) readJSON(path string) map[string]any {
	body, err := s.read(path)
	if err != nil {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil
	}
	document, _ := asObject(decoded)
	return document
}

func (s Source) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if s.Run != nil {
		return s.Run(ctx, name, args...)
	}
	ctx, cancel := context.WithTimeoutCause(ctx, commandWait, SaidError{Text: "No answer in 10 s."})
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = nil
	return command.Output()
}

func (s Source) shim(name string) string {
	path := filepath.Join(s.userHome(), ".local", "share", "mise", "shims", name)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return path
}

func (s Source) userHome() string {
	if s.UserHome != nil {
		return s.UserHome()
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func preferDotEnv(dotenv map[string]string, key, environ string) (string, string) {
	raw := strings.TrimSpace(dotenv[key])
	if strings.HasPrefix(raw, "op://") {
		raw = ""
	}
	if raw != "" {
		return raw, "Hermes .env (" + key + ")"
	}
	return strings.TrimSpace(environ), key + " environment variable"
}

func firstSecret(dotenv map[string]string, env func(string) string, keys ...string) (string, string) {
	for _, key := range keys {
		if value, location := preferDotEnv(dotenv, key, env(key)); value != "" {
			return value, location
		}
	}
	return "", ""
}

func requirePlanToken(token, location string) (string, string, error) {
	if claudeOAuth(token) {
		return token, location, nil
	}
	return "", "", MissingError{Text: "Claude's plan is only reported for a Claude sign-in, not a Console API key."}
}

func claudeOAuth(token string) bool {
	if token == "" {
		return false
	}
	for _, prefix := range []string{"sk-ant-api", "sk-ant-usr", "sk-ant-svc", "sk-ant-admin"} {
		if strings.HasPrefix(token, prefix) {
			return false
		}
	}
	return strings.HasPrefix(token, "sk-ant-") || strings.HasPrefix(token, "eyJ") || strings.HasPrefix(token, "cc-")
}

func parseDotEnv(text string) map[string]string {
	values := make(map[string]string)
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line, _ = strings.CutPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			quote := value[0]
			if quoted(quote) && closesQuote(value, quote) {
				value = value[1 : len(value)-1]
			}
		}
		values[key] = value
	}
	return values
}

func accessToken(document map[string]any, ids ...string) string {
	section, ok := asObject(document["providers"])
	if !ok {
		section = document
	}
	for _, id := range ids {
		state, ok := asObject(section[id])
		if !ok {
			continue
		}
		if tokens, ok := asObject(state["tokens"]); ok {
			if token, ok := asText(tokens["access_token"]); ok {
				return token
			}
		}
		if token, ok := asText(state["access_token"]); ok {
			return token
		}
	}
	return ""
}

func providerPresent(document map[string]any, ids ...string) bool {
	section, ok := asObject(document["providers"])
	if !ok {
		return false
	}
	for _, id := range ids {
		if _, ok := asObject(section[id]); ok {
			return true
		}
	}
	return false
}

func poolEntries(document map[string]any, provider string) ([]map[string]any, bool) {
	pool, ok := asObject(document["credential_pool"])
	if !ok {
		return nil, false
	}
	entries, ok := pool[provider].([]any)
	if !ok || noEntries(entries) {
		return nil, false
	}
	return asRecords(entries), true
}

func firstToken(entries []map[string]any) string {
	for _, entry := range entries {
		if token, ok := asText(entry["access_token"]); ok {
			return token
		}
	}
	return ""
}

func oauthAccessToken(document map[string]any, wrapper, field string) string {
	if document == nil {
		return ""
	}
	source := document
	if wrapper != "" {
		nested, ok := asObject(document[wrapper])
		if !ok {
			return ""
		}
		source = nested
	}
	token, _ := asText(source[field])
	return token
}

func blank(value string) bool { return value == "" }

func nonempty(value string) bool { return value != "" }

func quoted(quote byte) bool {
	return quote == '"' || singleQuote(quote)
}

func singleQuote(quote byte) bool { return quote == '\'' }

func closesQuote(value string, quote byte) bool { return value[len(value)-1] == quote }

func noEntries(entries []any) bool { return len(entries) == 0 }

func expandHome(path, home string) string {
	path = os.ExpandEnv(path)
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
