package config

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

// ResponseAPI is the only API an automatic review can call.
const ResponseAPI = "openai-responses"

// securityBinary is the only command a models file can run. Values never
// reach a shell, so shell metacharacters stay literal arguments.
var securityBinary = "/usr/bin/security"

// commandTimeout leaves room for a keychain prompt or a slow secret manager.
const commandTimeout = 30 * time.Second

// Models is a subset of the Pi models file schema. Only the fields an
// automatic review needs are read; every other field of the schema stays
// with Pi. Unlike Pi, '!'-values may only run /usr/bin/security.
type Models struct {
	Providers map[string]Provider `json:"providers"`
}

// Provider describes one model provider.
type Provider struct {
	Name    string            `json:"name,omitempty"`
	BaseURL string            `json:"baseUrl,omitempty"`
	APIKey  string            `json:"apiKey,omitempty"`
	API     string            `json:"api,omitempty"`
	OAuth   string            `json:"oauth,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Models  []Model           `json:"models,omitempty"`
}

// Model describes one model of a provider.
type Model struct {
	ID        string            `json:"id"`
	API       string            `json:"api,omitempty"`
	BaseURL   string            `json:"baseUrl,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	MaxTokens int               `json:"maxTokens,omitempty"`
}

// Resolved is the merged provider and model configuration for one selector.
// Values that resolve like an API key stay unexpanded until the caller picks
// them over the environment.
type Resolved struct {
	Provider  string
	Model     string
	BaseURL   string
	APIKey    string
	API       string
	OAuth     string
	MaxTokens int
	Headers   map[string]string
}

// LoadModels reads a models file. A missing file describes no models.
func LoadModels(path string) (Models, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Models{}, nil
	}
	if err != nil {
		return Models{}, fmt.Errorf("read models file: %w", err)
	}
	var models Models
	if err := json.Unmarshal(data, &models); err != nil {
		return Models{}, fmt.Errorf("read models file %s: %w", path, err)
	}
	return models, nil
}

// Count reports how many models the file describes.
func (m Models) Count() int {
	count := 0
	for _, provider := range m.Providers {
		count += len(provider.Models)
	}
	return count
}

// Only returns the selector of the sole model, so a file with one model needs
// no model selection.
func (m Models) Only() (string, bool) {
	selectors := []string{}
	for name, provider := range m.Providers {
		for _, model := range provider.Models {
			selectors = append(selectors, name+"/"+model.ID)
		}
	}
	if len(selectors) != 1 {
		return "", false
	}
	return selectors[0], true
}

// Resolve selects one model by "provider/model" or an exact model ID that only
// one provider offers.
func (m Models) Resolve(selector string) (Resolved, error) {
	if name, id, ok := strings.Cut(selector, "/"); ok {
		if provider, found := m.Providers[name]; found {
			for _, model := range provider.Models {
				if model.ID == id {
					return resolveModel(name, provider, model), nil
				}
			}
			return Resolved{}, fmt.Errorf("models file has no model %q for provider %q", id, name)
		}
	}
	matches := []Resolved{}
	for _, name := range slices.Sorted(maps.Keys(m.Providers)) {
		provider := m.Providers[name]
		for _, model := range provider.Models {
			if model.ID == selector {
				matches = append(matches, resolveModel(name, provider, model))
			}
		}
	}
	switch len(matches) {
	case 0:
		return Resolved{}, fmt.Errorf("models file has no model %q", selector)
	case 1:
		return matches[0], nil
	default:
		providers := make([]string, 0, len(matches))
		for _, match := range matches {
			providers = append(providers, match.Provider)
		}
		return Resolved{}, fmt.Errorf("model %q is offered by providers %s; select it as provider/model", selector, strings.Join(providers, ", "))
	}
}

func resolveModel(name string, provider Provider, model Model) Resolved {
	resolved := Resolved{
		Provider:  name,
		Model:     model.ID,
		BaseURL:   provider.BaseURL,
		APIKey:    provider.APIKey,
		API:       provider.API,
		OAuth:     provider.OAuth,
		MaxTokens: model.MaxTokens,
		Headers:   map[string]string{},
	}
	if model.BaseURL != "" {
		resolved.BaseURL = model.BaseURL
	}
	if model.API != "" {
		resolved.API = model.API
	}
	maps.Copy(resolved.Headers, provider.Headers)
	maps.Copy(resolved.Headers, model.Headers)
	return resolved
}

// Validate rejects a provider this review cannot call.
func (r Resolved) Validate() error {
	if r.OAuth != "" {
		return fmt.Errorf("provider %q uses the %q OAuth flow; an automatic review requires an API key", r.Provider, r.OAuth)
	}
	if r.API != ResponseAPI {
		return fmt.Errorf("model %s/%s uses api %q; an automatic review requires %q", r.Provider, r.Model, r.API, ResponseAPI)
	}
	if r.MaxTokens < 0 {
		return fmt.Errorf("model %s/%s has a negative maxTokens", r.Provider, r.Model)
	}
	return nil
}

// Values expands header values the way Value expands an API key.
func Values(values map[string]string) (map[string]string, error) {
	resolved := make(map[string]string, len(values))
	for _, name := range slices.Sorted(maps.Keys(values)) {
		value, err := Value(values[name])
		if err != nil {
			return nil, fmt.Errorf("header %s: %w", name, err)
		}
		resolved[name] = value
	}
	return resolved, nil
}

// Value expands a models file value. A leading "!security" runs
// /usr/bin/security without a shell, "$NAME" and "${NAME}" read the
// environment, and "$$" and "$!" produce a literal dollar or exclamation
// mark. Any other leading "!" is rejected, so a models file cannot run an
// arbitrary command.
func Value(value string) (string, error) {
	if rest, ok := strings.CutPrefix(value, "!"); ok {
		return securityOutput(value, rest)
	}
	var expanded strings.Builder
	for i := 0; i < len(value); {
		if value[i] != '$' {
			expanded.WriteByte(value[i])
			i++
			continue
		}
		if i+1 < len(value) && (value[i+1] == '$' || value[i+1] == '!') {
			expanded.WriteByte(value[i+1])
			i += 2
			continue
		}
		end := i + 1
		name := ""
		if i+1 < len(value) && value[i+1] == '{' {
			close := strings.IndexByte(value[i+2:], '}')
			if close < 0 {
				return "", fmt.Errorf("unterminated variable in %q", value)
			}
			name = value[i+2 : i+2+close]
			end = i + 2 + close + 1
		} else {
			for end < len(value) && isVariableByte(value[end], end == i+1) {
				end++
			}
			if end == i+1 {
				expanded.WriteByte('$')
				i++
				continue
			}
			name = value[i+1 : end]
		}
		environment, err := variable(name)
		if err != nil {
			return "", err
		}
		expanded.WriteString(environment)
		i = end
	}
	return expanded.String(), nil
}

func isVariableByte(b byte, first bool) bool {
	if b == '_' || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') {
		return true
	}
	return !first && b >= '0' && b <= '9'
}

func variable(name string) (string, error) {
	value, ok := os.LookupEnv(name)
	if !ok || value == "" {
		return "", fmt.Errorf("environment variable %s is not set", name)
	}
	return value, nil
}

func securityOutput(value, rest string) (string, error) {
	arguments, ok := strings.CutPrefix(rest, "security")
	if !ok || (len(arguments) > 0 && arguments[0] != ' ' && arguments[0] != '\t') {
		return "", fmt.Errorf("unsupported command in %q: only \"!security ...\" may run %s", value, securityBinary)
	}
	args, err := splitSecurityArgs(arguments)
	if err != nil {
		return "", err
	}
	if len(args) == 0 {
		return "", fmt.Errorf("\"!security\" requires arguments")
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, securityBinary, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return "", fmt.Errorf("security %q: %w: %s", strings.Join(args, " "), err, message)
		}
		return "", fmt.Errorf("security %q: %w", strings.Join(args, " "), err)
	}
	resolved := strings.TrimRight(string(output), "\r\n")
	if resolved == "" {
		return "", fmt.Errorf("security %q returned no value", strings.Join(args, " "))
	}
	return resolved, nil
}

// splitSecurityArgs splits on whitespace with single quotes, double quotes,
// and backslash escapes. It never interprets command substitution, globs, or
// redirections, so every argument stays a literal.
func splitSecurityArgs(rest string) ([]string, error) {
	var args []string
	var current strings.Builder
	var inSingle, inDouble, escaped, hasContent bool
	flush := func() {
		if hasContent {
			args = append(args, current.String())
			current.Reset()
			hasContent = false
		}
	}
	for _, r := range rest {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && !inSingle:
			escaped = true
			hasContent = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
			hasContent = true
		case r == '"' && !inSingle:
			inDouble = !inDouble
			hasContent = true
		case inSingle || inDouble:
			current.WriteRune(r)
		case r == ' ' || r == '\t' || r == '\n':
			flush()
		default:
			current.WriteRune(r)
			hasContent = true
		}
	}
	if escaped || inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quote in \"!security%s\"", rest)
	}
	flush()
	return args, nil
}
