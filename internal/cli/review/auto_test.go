package cli

import (
	"bytes"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	reviewroot "github.com/nnutter/timber/internal/cli/review/root"
	coreconfig "github.com/nnutter/timber/internal/timber/config"
	"github.com/nnutter/timber/internal/timber/review"
)

func TestAutoCommandDryRunThenRecordsOpenAIVerdict(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/responses" {
			t.Errorf("unexpected API path: %s", r.URL.Path)
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if !strings.Contains(string(body), `"model":"test-model"`) {
			t.Errorf("request did not use selected model: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, err = io.WriteString(w, `{"id":"resp_cli","object":"response","status":"completed","model":"test-model","output":[{"id":"msg_cli","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"{\"decision\":\"approve\",\"reason\":\"Only a private name changes.\",\"evidence\":[\"code.go:2 preserves the constant value\"]}","annotations":[]}]}]}`)
		if err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	t.Setenv("OPENAI_API_KEY", "test-placeholder")
	t.Setenv("OPENAI_BASE_URL", server.URL+"/v1/")
	dir := featureRepo(t)
	file := filepath.Join(dir, "code.go")
	require.NoError(t, os.WriteFile(file, []byte("package example\nconst old = 1\n"), 0o600))
	git(t, dir, "add", "code.go")
	git(t, dir, "commit", "-m", "Add constant")
	require.NoError(t, os.WriteFile(file, []byte("package example\nconst renamed = 1\n"), 0o600))
	git(t, dir, "commit", "-am", "Refactor: Rename private constant")
	out, _, err := execute(t, dir, "auto", "--model", "test-model", "--start", "HEAD", "--dry-run", "--json")
	require.NoError(t, err)
	var dry review.AutoResult
	require.NoError(t, json.Unmarshal([]byte(out), &dry))
	require.Equal(t, "auto", dry.Command)
	require.True(t, dry.DryRun)
	require.Len(t, dry.Entries, 1)
	require.False(t, dry.Entries[0].Approved)
	require.Equal(t, "approve", dry.Entries[0].Evaluation.Decision)
	_, err = os.Stat(dry.View.Path)
	require.ErrorIs(t, err, os.ErrNotExist)
	out, progress, err := execute(t, dir, "auto", "--model", "test-model", "--start", "HEAD", "--dry-run")
	require.NoError(t, err)
	require.Contains(t, out, "Would approve")
	require.Contains(t, progress, "Stack 1/1: assessing")
	require.Contains(t, out, "Only a private name changes.")
	out, _, err = execute(t, dir, "auto", "--model", "test-model", "--start", "HEAD")
	require.NoError(t, err)
	require.Contains(t, out, "Auto-Approved")
	out, _, err = execute(t, dir, "status", "--json")
	require.NoError(t, err)
	var status review.Result
	require.NoError(t, json.Unmarshal([]byte(out), &status))
	require.Equal(t, "model", status.Entries[2].Attempt.ApprovalSource)
	require.Equal(t, "resp_cli", status.Entries[2].Attempt.Automatic.ResponseID)
	out, _, err = execute(t, dir, "status")
	require.NoError(t, err)
	require.Contains(t, out, "Auto-Approved")
	out, _, err = execute(t, dir, "auto", "--model", "test-model", "--start", "HEAD")
	require.NoError(t, err)
	require.Contains(t, out, "Model result recorded: Only a private name changes.")
	require.Equal(t, 3, requests)
}

// refactorRepo adds one eligible Refactor: stack on top of the feature branch.
func refactorRepo(t *testing.T) string {
	t.Helper()
	dir := featureRepo(t)
	file := filepath.Join(dir, "code.go")
	require.NoError(t, os.WriteFile(file, []byte("package example\nconst old = 1\n"), 0o600))
	git(t, dir, "add", "code.go")
	git(t, dir, "commit", "-m", "Refactor: Add constant")
	return dir
}

const abstainVerdict = `{"id":"resp_models","object":"response","status":"completed","model":"cfg-model","output":[{"id":"msg","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"{\"decision\":\"needs-human-review\",\"reason\":\"uncertain\",\"evidence\":[]}","annotations":[]}]}]}`

// executeWithModels runs the review tree with an in-memory models file.
func executeWithModels(t *testing.T, dir string, models coreconfig.Models, args ...string) (string, string, error) {
	t.Helper()
	var output, terminal bytes.Buffer
	root := reviewroot.New(review.Service{Repository: review.Repository{Dir: dir}}, strings.NewReader(""), &output, &terminal, false, func() (coreconfig.Models, error) { return models, nil })
	root.SetArgs(args)
	root.SetIn(strings.NewReader(""))
	root.SetOut(&output)
	root.SetErr(&terminal)
	err := root.Execute()
	return output.String(), terminal.String(), err
}

func modelsFile(baseURL, apiKey string, maxTokens int) coreconfig.Models {
	return coreconfig.Models{Providers: map[string]coreconfig.Provider{
		"test": {
			BaseURL: baseURL,
			APIKey:  apiKey,
			API:     coreconfig.ResponseAPI,
			Models:  []coreconfig.Model{{ID: "cfg-model", MaxTokens: maxTokens}},
		},
	}}
}

func TestAutoResolvesTheOnlyModelInTheModelsFile(t *testing.T) {
	var authorization, model string
	var maxTokens float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Error(err)
			return
		}
		model, _ = request["model"].(string)
		maxTokens, _ = request["max_output_tokens"].(float64)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, abstainVerdict)
	}))
	defer server.Close()
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("TIMBER_REVIEW_MODEL", "")
	out, _, err := executeWithModels(t, refactorRepo(t), modelsFile(server.URL+"/v1/", "key-from-command", 1024), "auto", "--start", "HEAD", "--dry-run")
	require.NoError(t, err)
	require.Contains(t, out, "Needs human review")
	require.Equal(t, "cfg-model", model)
	require.Equal(t, "Bearer key-from-command", authorization)
	require.InDelta(t, 1024, maxTokens, 0)
}

func TestAutoPrefersTheEnvironmentAndFlagsOverTheModelsFile(t *testing.T) {
	var authorization, model string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Error(err)
			return
		}
		model, _ = request["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, abstainVerdict)
	}))
	defer server.Close()
	t.Setenv("OPENAI_API_KEY", "env-key")
	t.Setenv("OPENAI_BASE_URL", server.URL+"/v1/")
	t.Setenv("TIMBER_REVIEW_MODEL", "cfg-model")
	// The file names another endpoint and key; the environment overrides both.
	models := modelsFile("http://127.0.0.1:1/v1/", "file-key", 0)
	provider := models.Providers["test"]
	provider.Models = append(provider.Models, coreconfig.Model{ID: "other-model"})
	models.Providers["test"] = provider
	_, _, err := executeWithModels(t, refactorRepo(t), models, "auto", "--start", "HEAD", "--dry-run", "--model", "test/other-model")
	require.NoError(t, err)
	require.Equal(t, "other-model", model)
	require.Equal(t, "Bearer env-key", authorization)
}

func TestAutoRejectsAProviderItCannotCall(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-placeholder")
	models := coreconfig.Models{Providers: map[string]coreconfig.Provider{
		"test": {BaseURL: "https://example/v1", API: "openai-completions", Models: []coreconfig.Model{{ID: "cfg-model"}}},
	}}
	_, _, err := executeWithModels(t, refactorRepo(t), models, "auto", "--start", "HEAD", "--dry-run")
	require.ErrorContains(t, err, `requires "openai-responses"`)
}

func TestAutoRequiresAModelWhenTheModelsFileHasNone(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-placeholder")
	t.Setenv("TIMBER_REVIEW_MODEL", "")
	models := coreconfig.Models{Providers: map[string]coreconfig.Provider{
		"test": {BaseURL: "https://example/v1", API: coreconfig.ResponseAPI},
	}}
	_, _, err := executeWithModels(t, refactorRepo(t), models, "auto", "--start", "HEAD", "--dry-run")
	require.ErrorContains(t, err, "models file that describes a model")
}
