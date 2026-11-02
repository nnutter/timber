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
	"github.com/stretchr/testify/assert"
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
		if !assert.NoError(t, err) {
			return
		}
		var request map[string]any
		if !assert.NoError(t, json.Unmarshal(body, &request)) {
			return
		}
		assert.Equal(t, false, request["store"])
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			tools, _ := request["tools"].([]any)
			assert.Len(t, tools, 2)
			_, err = io.WriteString(w, `{"id":"one","model":"m","status":"completed","output":[{"type":"reasoning","id":"rs1","summary":[],"encrypted_content":"opaque"},{"type":"function_call","id":"fc1","call_id":"call1","name":"read_file","arguments":"{\"revision\":\"base\",\"path\":\"a.go\"}","status":"completed"}]}`)
			assert.NoError(t, err)
			return
		}
		items, _ := request["input"].([]any)
		if !assert.Len(t, items, 4) {
			return
		}
		reasoning, _ := items[1].(map[string]any)
		assert.Equal(t, "reasoning", reasoning["type"], "reasoning item = %#v", reasoning)
		result, _ := items[3].(map[string]any)
		assert.Equal(t, "function_call_output", result["type"], "tool result = %#v", result)
		assert.Equal(t, "call1", result["call_id"], "tool result = %#v", result)
		assert.Equal(t, "base:a.go", result["output"], "tool result = %#v", result)
		_, err = io.WriteString(w, `{"id":"two","model":"m","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"{\"decision\":\"approve\",\"reason\":\"same\",\"evidence\":[\"file\"]}"}]}]}`)
		assert.NoError(t, err)
	}))
	defer server.Close()
	o := OpenAI{Client: openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(server.URL+"/")), Model: "m"}
	v, err := o.Evaluate(t.Context(), Input{}, testReader{})
	require.NoError(t, err)
	require.Equal(t, "approve", v.Decision)
	require.Equal(t, 2, requests)
}
