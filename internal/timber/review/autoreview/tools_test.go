package autoreview

import (
	"context"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"
)

type testReader struct{}

func (testReader) ReadFile(_ context.Context, rev, path string) (string, error) {
	return rev + ":" + path, nil
}

func (testReader) Search(_ context.Context, rev, query string) (string, error) {
	return rev + ":" + query, nil
}

func TestOpenAIToolContinuation(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
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
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			tools, _ := request["tools"].([]any)
			if len(tools) != 2 {
				t.Errorf("tools = %d, want 2", len(tools))
			}
			_, err = io.WriteString(w, `{"id":"one","model":"m","status":"completed","output":[{"type":"reasoning","id":"rs1","summary":[],"encrypted_content":"opaque"},{"type":"function_call","id":"fc1","call_id":"call1","name":"read_file","arguments":"{\"revision\":\"base\",\"path\":\"a.go\"}","status":"completed"}]}`)
			if err != nil {
				t.Error(err)
			}
			return
		}
		items, _ := request["input"].([]any)
		if len(items) != 4 {
			t.Errorf("input items = %d, want 4", len(items))
			return
		}
		reasoning, _ := items[1].(map[string]any)
		if reasoning["type"] != "reasoning" {
			t.Errorf("reasoning item = %#v", reasoning)
		}
		result, _ := items[3].(map[string]any)
		if result["type"] != "function_call_output" || result["call_id"] != "call1" || result["output"] != "base:a.go" {
			t.Errorf("tool result = %#v", result)
		}
		_, err = io.WriteString(w, `{"id":"two","model":"m","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"{\"decision\":\"approve\",\"reason\":\"same\",\"evidence\":[\"file\"]}"}]}]}`)
		if err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	o := OpenAI{Client: openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(server.URL+"/")), Model: "m"}
	v, err := o.Evaluate(t.Context(), Input{}, testReader{})
	require.NoError(t, err)
	require.Equal(t, "approve", v.Decision)
	require.Equal(t, 2, requests)
}
