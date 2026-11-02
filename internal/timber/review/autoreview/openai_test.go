package autoreview

import (
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIFinalVerdict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/responses", r.URL.Path, "unexpected API path")
		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			return
		}
		var request map[string]any
		if !assert.NoError(t, json.Unmarshal(body, &request)) {
			return
		}
		assert.Equal(t, false, request["store"])
		assert.InDelta(t, DefaultMaxOutputTokens, request["max_output_tokens"], 0)
		text, _ := request["text"].(map[string]any)
		format, _ := text["format"].(map[string]any)
		assert.Equal(t, "json_schema", format["type"])
		assert.Equal(t, true, format["strict"])
		w.Header().Set("Content-Type", "application/json")
		_, err = io.WriteString(w, `{"id":"resp_1","model":"actual-model","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"{\"decision\":\"approve\",\"reason\":\"equivalent\",\"evidence\":[\"same output\"]}"}]}]}`)
		assert.NoError(t, err)
	}))
	defer server.Close()
	client := openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(server.URL+"/"))
	evaluator := OpenAI{Client: client, Model: "test-model"}
	verdict, err := evaluator.Evaluate(t.Context(), Input{Subject: "Refactor: example"}, nil)
	require.NoError(t, err)
	require.Equal(t, "approve", verdict.Decision)
	require.Equal(t, "actual-model", verdict.Model)
	require.Equal(t, "resp_1", verdict.ResponseID)
}

func TestOpenAISendsConfiguredOutputBudget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			return
		}
		var request map[string]any
		if !assert.NoError(t, json.Unmarshal(body, &request)) {
			return
		}
		assert.InDelta(t, 2048, request["max_output_tokens"], 0)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_1","model":"m","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"{\"decision\":\"needs-human-review\",\"reason\":\"uncertain\",\"evidence\":[]}"}]}]}`)
	}))
	defer server.Close()
	evaluator := OpenAI{Client: openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(server.URL+"/")), Model: "m", MaxOutputTokens: 2048}
	_, err := evaluator.Evaluate(t.Context(), Input{}, nil)
	require.NoError(t, err)
}
