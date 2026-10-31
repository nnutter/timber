package autoreview

import (
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"
)

func TestOpenAIFinalVerdict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("unexpected API path: %s", r.URL.Path)
		}
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
		if request["store"] != false {
			t.Errorf("store = %#v, want false", request["store"])
		}
		if request["max_output_tokens"] != float64(DefaultMaxOutputTokens) {
			t.Errorf("max_output_tokens = %#v, want %d", request["max_output_tokens"], DefaultMaxOutputTokens)
		}
		text, _ := request["text"].(map[string]any)
		format, _ := text["format"].(map[string]any)
		if format["type"] != "json_schema" || format["strict"] != true {
			t.Errorf("response format = %#v, want strict json_schema", format)
		}
		w.Header().Set("Content-Type", "application/json")
		_, err = io.WriteString(w, `{"id":"resp_1","model":"actual-model","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"{\"decision\":\"approve\",\"reason\":\"equivalent\",\"evidence\":[\"same output\"]}"}]}]}`)
		if err != nil {
			t.Error(err)
		}
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
		if err != nil {
			t.Error(err)
			return
		}
		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Error(err)
			return
		}
		if request["max_output_tokens"] != float64(2048) {
			t.Errorf("max_output_tokens = %#v, want 2048", request["max_output_tokens"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_1","model":"m","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"{\"decision\":\"needs-human-review\",\"reason\":\"uncertain\",\"evidence\":[]}"}]}]}`)
	}))
	defer server.Close()
	evaluator := OpenAI{Client: openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(server.URL+"/")), Model: "m", MaxOutputTokens: 2048}
	_, err := evaluator.Evaluate(t.Context(), Input{}, nil)
	require.NoError(t, err)
}
